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

package deviceplugin

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	resourceNamePrefix = "networkservicemesh.io/"
	kubeletNotifyDelay = 30 * time.Second
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
}

// StartServer creates a new SR-IOV forwarder device plugin server and starts it
func StartServer(ctx context.Context, config *ServerConfig, manager Manager) error {
	logFmt := "StartServer: %v"

	s := &devicePluginServer{
		resourceName:  resourceNamePrefix + config.ResourceName,
		resourceCount: config.ResourceCount,
		hostBaseDir:   config.HostBaseDir,
		hostPathEnv:   config.HostPathEnv,
		ctx:           ctx,
	}

	logrus.Infof(logFmt, "starting server")
	socket, err := manager.StartDeviceServer(s.ctx, s)
	if err != nil {
		logrus.Errorf(logFmt, "error starting server")
		return err
	}

	logrus.Infof(logFmt, "registering server")
	if err := s.register(manager, socket); err != nil {
		logrus.Errorf(logFmt, "error registering server")
		return err
	}

	if resetCh, err := manager.MonitorKubeletRestart(s.ctx); err == nil {
		go func() {
			logrus.Infof(logFmt, "start monitoring kubelet restart")
			defer logrus.Infof(logFmt, "stop monitoring kubelet restart")
			for {
				select {
				case <-s.ctx.Done():
					return
				case _, ok := <-resetCh:
					if !ok {
						return
					}
					logrus.Infof(logFmt, "re registering server")
					if err := s.register(manager, socket); err != nil {
						logrus.Errorf(logFmt, fmt.Sprint("error re registering server: ", err))
						return
					}
				}
			}
		}()
	} else {
		logrus.Warnf(logFmt, "error monitoring kubelet restart")
	}

	return nil
}

func (s *devicePluginServer) register(manager Manager, socket string) error {
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
	logFmt := "devicePluginServer(ListAndWatch): %v"

	resp := &pluginapi.ListAndWatchResponse{
		Devices: make([]*pluginapi.Device, s.resourceCount),
	}
	for i := 0; i < s.resourceCount; i++ {
		resp.Devices[i] = &pluginapi.Device{
			ID:     fmt.Sprintf("%s/%v", s.resourceName, i),
			Health: pluginapi.Healthy,
		}
	}

	for {
		if err := server.Send(resp); err != nil {
			logrus.Errorf(logFmt, "server unavailable")
			return err
		}
		select {
		case <-s.ctx.Done():
			logrus.Infof(logFmt, "server stopped")
			return nil
		case <-time.After(kubeletNotifyDelay):
		}
	}
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
