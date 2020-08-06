// Copyright (c) 2020 Doc.ai and/or its affiliates.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package deviceplugin provides tools for setting up device plugin server
package deviceplugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	podresources "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"
)

const (
	resourceNamePrefix = "networkservicemesh.io/"
	kubeletNotifyDelay = 30 * time.Second
	vfioDir            = "/dev/vfio"

	deviceFree  deviceState = 0
	deviceInUse deviceState = 2
)

// ServerConfig is a struct for configuring device plugin server
type ServerConfig struct {
	ResourceName  string
	ResourceCount int
	HostBaseDir   string
	HostPathEnv   string
}

// K8sManager is a bridge interface to the k8s API
type K8sManager interface {
	StartDeviceServer(ctx context.Context, deviceServer pluginapi.DevicePluginServer) (string, error)
	RegisterDeviceServer(ctx context.Context, request *pluginapi.RegisterRequest) error
	MonitorKubeletRestart(ctx context.Context) (chan bool, error)
	GetPodResourcesListerClient(ctx context.Context) (podresources.PodResourcesListerClient, error)
}

type devicePluginServer struct {
	ctx                  context.Context
	resourceName         string
	resourceCount        int
	resourcePollTimeout  time.Duration
	hostBaseDir          string
	hostPathEnv          string
	devices              map[string]*device
	updateDevicesCh      chan bool
	lock                 sync.Mutex
	resourceListerClient podresources.PodResourcesListerClient
}

type deviceState int32

type device struct {
	state         deviceState
	clientCleanup func()
}

func newDevicePluginServer(ctx context.Context, config *ServerConfig) *devicePluginServer {
	return &devicePluginServer{
		ctx:                 ctx,
		resourceName:        resourceNamePrefix + config.ResourceName,
		resourceCount:       config.ResourceCount,
		resourcePollTimeout: kubeletNotifyDelay,
		hostBaseDir:         config.HostBaseDir,
		hostPathEnv:         config.HostPathEnv,
		devices:             map[string]*device{},
		updateDevicesCh:     make(chan bool, 1),
	}
}

// StartServer creates a new SR-IOV forwarder device plugin server and starts it
func StartServer(ctx context.Context, config *ServerConfig, manager K8sManager) error {
	logEntry := log.Entry(ctx).WithField("devicePluginServer", "StartServer")

	s := newDevicePluginServer(ctx, config)

	logEntry.Info("get resource lister client")
	resourceListerClient, err := manager.GetPodResourcesListerClient(ctx)
	if err != nil {
		logEntry.Error("failed to get resource lister client")
		return err
	}
	s.resourceListerClient = resourceListerClient

	logEntry.Info("starting server")
	socket, err := manager.StartDeviceServer(s.ctx, s)
	if err != nil {
		logEntry.Error("error starting server")
		return err
	}

	logEntry.Info("registering server")
	if err := s.register(manager, socket); err != nil {
		logEntry.Error("error registering server")
		return err
	}

	if resetCh, err := manager.MonitorKubeletRestart(s.ctx); err == nil {
		go func() {
			logEntry.Infof("start monitoring kubelet restart")
			defer logEntry.Infof("stop monitoring kubelet restart")
			for {
				select {
				case <-s.ctx.Done():
					return
				case _, ok := <-resetCh:
					if !ok {
						return
					}
					logEntry.Info("re registering server")
					if err = s.register(manager, socket); err != nil {
						logEntry.Errorf("error re registering server: %+v", err)
						return
					}
				}
			}
		}()
	} else {
		logEntry.Warnf("error monitoring kubelet restart: %+v", err)
	}

	return nil
}

func (s *devicePluginServer) register(manager K8sManager, socket string) error {
	return manager.RegisterDeviceServer(s.ctx, &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     socket,
		ResourceName: s.resourceName,
	})
}

func (s *devicePluginServer) GetDevicePluginOptions(_ context.Context, _ *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func (s *devicePluginServer) ListAndWatch(_ *pluginapi.Empty, server pluginapi.DevicePlugin_ListAndWatchServer) error {
	logEntry := log.Entry(s.ctx).WithField("devicePluginServer", "ListAndWatch")

	for {
		resp, err := s.resourceListerClient.List(s.ctx, &podresources.ListPodResourcesRequest{})
		if err != nil {
			logEntry.Errorf("resourceListerClient unavailable: %+v", err)
			return err
		}

		s.updateDevices(s.respToDeviceIds(resp))

		if err := server.Send(s.listAndWatchResponse()); err != nil {
			logEntry.Errorf("server unavailable: %+v", err)
			return err
		}

		select {
		case <-s.ctx.Done():
			logEntry.Info("server stopped")
			return nil
		case <-time.After(s.resourcePollTimeout):
		case <-s.updateDevicesCh:
		}
	}
}

func (s *devicePluginServer) respToDeviceIds(resp *podresources.ListPodResourcesResponse) map[string]bool {
	deviceIds := map[string]bool{}
	for _, pod := range resp.PodResources {
		for _, container := range pod.Containers {
			for _, device := range container.Devices {
				if device.ResourceName == s.resourceName {
					for _, id := range device.DeviceIds {
						deviceIds[id] = true
					}
				}
			}
		}
	}
	return deviceIds
}

func (s *devicePluginServer) updateDevices(idsInUse map[string]bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	var freeCount int
	for id, device := range s.devices {
		if idsInUse[id] {
			device.state = deviceInUse
		} else if device.state != deviceFree {
			device.state--
		}

		if device.state == deviceFree {
			if device.clientCleanup != nil {
				device.clientCleanup()
				device.clientCleanup = nil
			}
			if freeCount < s.resourceCount {
				freeCount++
			} else {
				delete(s.devices, id)
			}
		}
	}

	for i := 0; freeCount < s.resourceCount; i++ {
		id := fmt.Sprintf("%s/%v", s.resourceName, i)
		if _, ok := s.devices[id]; !ok {
			s.devices[id] = &device{}
			freeCount++
		}
	}
}

func (s *devicePluginServer) listAndWatchResponse() *pluginapi.ListAndWatchResponse {
	var devices []*pluginapi.Device
	for id := range s.devices {
		devices = append(devices, &pluginapi.Device{
			ID:     id,
			Health: pluginapi.Healthy,
		})
	}
	return &pluginapi.ListAndWatchResponse{
		Devices: devices,
	}
}

func (s *devicePluginServer) Allocate(_ context.Context, request *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	response := &pluginapi.AllocateResponse{
		ContainerResponses: make([]*pluginapi.ContainerAllocateResponse, len(request.ContainerRequests)),
	}

	devices := map[string]*device{}
	for i, container := range request.ContainerRequests {
		hostPath := path.Join(s.hostBaseDir, uuid.New().String())

		for k, id := range container.DevicesIDs {
			devices[id] = &device{
				state: deviceInUse,
			}
			if k == 0 {
				devices[id].clientCleanup = func() { _ = os.RemoveAll(hostPath) }
			}
		}

		response.ContainerResponses[i] = &pluginapi.ContainerAllocateResponse{
			Envs: map[string]string{
				s.hostPathEnv: hostPath,
			},
			Mounts: []*pluginapi.Mount{{
				ContainerPath: vfioDir,
				HostPath:      hostPath,
				ReadOnly:      false,
			}},
		}
	}

	if err := s.useDevices(devices); err != nil {
		return nil, err
	}

	select {
	case s.updateDevicesCh <- true:
	default:
	}

	return response, nil
}

func (s *devicePluginServer) useDevices(devices map[string]*device) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	for id := range devices {
		if _, ok := s.devices[id]; !ok {
			return errors.New("trying to use non existing device")
		}
	}

	for id, device := range devices {
		if s.devices[id].clientCleanup != nil {
			s.devices[id].clientCleanup()
		}
		s.devices[id] = device
	}

	return nil
}

func (s *devicePluginServer) PreStartContainer(_ context.Context, _ *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}
