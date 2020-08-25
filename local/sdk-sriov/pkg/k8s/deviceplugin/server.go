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
	"sync"
	"time"

	"github.com/networkservicemesh/sdk/pkg/tools/log"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	podresources "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"
)

const (
	resourceNamePrefix = "networkservicemesh.io/"
)

// ServerConfig is a struct for configuring device plugin server
type ServerConfig struct {
	ResourceName        string
	ResourceCount       int
	ResourcePollTimeout time.Duration
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
	devices              map[string]bool
	updateDevicesCh      chan bool
	lock                 sync.Mutex
	resourceListerClient podresources.PodResourcesListerClient
}

// StartServer creates a new SR-IOV forwarder device plugin server and starts it
func StartServer(ctx context.Context, config *ServerConfig, manager K8sManager) (pluginapi.DevicePluginServer, error) {
	logEntry := log.Entry(ctx).WithField("devicePluginServer", "StartServer")

	s := &devicePluginServer{
		ctx:                 ctx,
		resourceName:        resourceNamePrefix + config.ResourceName,
		resourceCount:       config.ResourceCount,
		resourcePollTimeout: config.ResourcePollTimeout,
		devices:             map[string]bool{},
		updateDevicesCh:     make(chan bool, 1),
	}

	logEntry.Info("get resource lister client")
	resourceListerClient, err := manager.GetPodResourcesListerClient(ctx)
	if err != nil {
		logEntry.Error("failed to get resource lister client")
		return nil, err
	}
	s.resourceListerClient = resourceListerClient

	logEntry.Info("starting server")
	socket, err := manager.StartDeviceServer(s.ctx, s)
	if err != nil {
		logEntry.Error("error starting server")
		return nil, err
	}

	logEntry.Info("registering server")
	if err := manager.RegisterDeviceServer(s.ctx, &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     socket,
		ResourceName: s.resourceName,
	}); err != nil {
		logEntry.Error("error registering server")
		return nil, err
	}

	if err := s.monitorKubeletRestart(manager, socket); err != nil {
		logEntry.Warnf("error monitoring kubelet restart: %+v", err)
	}

	return s, nil
}

func (s *devicePluginServer) monitorKubeletRestart(manager K8sManager, socket string) error {
	logEntry := log.Entry(s.ctx).WithField("devicePluginServer", "monitorKubeletRestart")

	resetCh, err := manager.MonitorKubeletRestart(s.ctx)
	if err != nil {
		return err
	}

	go func() {
		logEntry.Info("start monitoring kubelet restart")
		defer logEntry.Info("stop monitoring kubelet restart")
		for {
			select {
			case <-s.ctx.Done():
				return
			case _, ok := <-resetCh:
				if !ok {
					return
				}
				logEntry.Info("re registering server")
				if err = manager.RegisterDeviceServer(s.ctx, &pluginapi.RegisterRequest{
					Version:      pluginapi.Version,
					Endpoint:     socket,
					ResourceName: s.resourceName,
				}); err != nil {
					logEntry.Errorf("error re registering server: %+v", err)
					return
				}
			}
		}
	}()

	return nil
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
			return s.ctx.Err()
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
	for id, free := range s.devices {
		switch {
		case idsInUse[id]:
			s.devices[id] = false
		case !free:
			s.devices[id] = true
		default:
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
			s.devices[id] = true
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

	devices := map[string]bool{}
	for i, container := range request.ContainerRequests {
		for _, id := range container.DevicesIDs {
			devices[id] = false
		}

		response.ContainerResponses[i] = &pluginapi.ContainerAllocateResponse{}
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

func (s *devicePluginServer) useDevices(devices map[string]bool) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	for id := range devices {
		if _, ok := s.devices[id]; !ok {
			return errors.New("trying to use non existing device")
		}
	}

	for id, device := range devices {
		s.devices[id] = device
	}

	return nil
}

func (s *devicePluginServer) PreStartContainer(_ context.Context, _ *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}
