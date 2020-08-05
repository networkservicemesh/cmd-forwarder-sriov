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
	"fmt"
	"path"

	"github.com/google/uuid"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/k8s"
)

const (
	resourceNamePrefix = "networkservicemesh.io/"
	vfioDir            = "/dev/vfio"
)

// ServerConfig is a struct for configuring device plugin server
type ServerConfig struct {
	ResourceName  string
	ResourceCount int
	HostBaseDir   string
	HostPathEnv   string
}

type devicePluginServer struct {
	resourceName  string
	resourceCount int
	hostBaseDir   string
	hostPathEnv   string
	ctx           context.Context
	deviceMonitor DeviceMonitor
	freeDevices   map[string]bool
}

func newDevicePluginServer(ctx context.Context, config *ServerConfig) *devicePluginServer {
	return &devicePluginServer{
		resourceName:  resourceNamePrefix + config.ResourceName,
		resourceCount: config.ResourceCount,
		hostBaseDir:   config.HostBaseDir,
		hostPathEnv:   config.HostPathEnv,
		ctx:           ctx,
		freeDevices:   map[string]bool{},
	}
}

// StartServer creates a new SR-IOV forwarder device plugin server and starts it
func StartServer(ctx context.Context, config *ServerConfig, manager k8s.Manager) error {
	logEntry := log.Entry(ctx).WithField("devicePluginServer", "StartServer")

	s := newDevicePluginServer(ctx, config)

	logEntry.Info("starting device monitor")
	deviceMonitor, err := newDeviceMonitor(ctx, manager)
	if err != nil {
		logEntry.Error("error starting device monitor")
		return err
	}
	s.deviceMonitor = deviceMonitor

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

func (s *devicePluginServer) register(manager k8s.Manager, socket string) error {
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

	usedDevicesCh, errCh := s.deviceMonitor.Monitor(s.resourceName)

	var usedDevices []string
	for {
		if err := server.Send(&pluginapi.ListAndWatchResponse{
			Devices: s.deviceList(usedDevices),
		}); err != nil {
			logEntry.Error("server unavailable")
			return err
		}
		select {
		case <-s.ctx.Done():
			logEntry.Info("server stopped")
			return nil
		case err, ok := <-errCh:
			if ok {
				logEntry.Error("device monitor unavailable")
				return err
			}
			return nil
		case usedDevices = <-usedDevicesCh:
		}
	}
}

func (s *devicePluginServer) deviceList(usedDevices []string) []*pluginapi.Device {
	for id := range s.freeDevices {
		s.freeDevices[id] = true
	}

	for _, id := range usedDevices {
		s.freeDevices[id] = false
	}

	freeCount := len(s.freeDevices) - len(usedDevices)
	for i := 0; freeCount != s.resourceCount; i++ {
		id := fmt.Sprintf("%s/%v", s.resourceName, i)
		if freeCount < s.resourceCount {
			if _, ok := s.freeDevices[id]; !ok {
				s.freeDevices[id] = true
				freeCount++
			}
		} else {
			if isFree, ok := s.freeDevices[id]; ok && isFree {
				delete(s.freeDevices, id)
				freeCount--
			}
		}
	}

	var devices []*pluginapi.Device
	for id := range s.freeDevices {
		devices = append(devices, &pluginapi.Device{
			ID:     id,
			Health: pluginapi.Healthy,
		})
	}
	return devices
}

func (s *devicePluginServer) Allocate(_ context.Context, request *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	response := &pluginapi.AllocateResponse{
		ContainerResponses: make([]*pluginapi.ContainerAllocateResponse, len(request.ContainerRequests)),
	}

	for i := range request.ContainerRequests {
		hostPath := path.Join(s.hostBaseDir, uuid.New().String())
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

	return response, nil
}

func (s *devicePluginServer) PreStartContainer(_ context.Context, _ *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}
