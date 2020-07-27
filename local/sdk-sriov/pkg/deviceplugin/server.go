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
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	resourceCount      = 10
	kubeletNotifyDelay = 30 * time.Second

	vfioDir     = "/sys/dev/vfio"
	hostBaseDir = "/networkservicemesh/sriov/vfio"
	hostPathEnv = "SRIOV_HOST_PATH"
)

// Server is a SR-IOV forwarder device plugin server
type Server interface {
	// Start starts server with the given context
	Start(ctx context.Context) error
	// Stop stops server
	Stop() error
}

type devicePluginServer struct {
	resourceName string
	ctx          context.Context
	stop         context.CancelFunc
}

// NewServer creates a new SR-IOV forwarder device plugin server
func NewServer(resourceName string) Server {
	return &devicePluginServer{
		resourceName: resourceName,
	}
}

func (s *devicePluginServer) Start(parentCtx context.Context) error {
	logFmt := "devicePluginServer(Start): %v"

	var ctx context.Context
	s.ctx, s.stop = context.WithCancel(parentCtx)

	socket, err := StartDeviceServer(ctx, s)
	if err != nil {
		logrus.Errorf(logFmt, "error starting server")
		return err
	}

	if err := RegisterDeviceServer(ctx, &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     socket,
		ResourceName: s.resourceName,
	}); err != nil {
		logrus.Errorf(logFmt, "error registering server")
		return err
	}

	return nil
}

func (s *devicePluginServer) Stop() error {
	if s.stop == nil {
		return errors.New("devicePluginServer(Stop): server is not running")
	}
	s.stop()
	return nil
}

func (s *devicePluginServer) GetDevicePluginOptions(_ context.Context, _ *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func (s *devicePluginServer) ListAndWatch(_ *pluginapi.Empty, server pluginapi.DevicePlugin_ListAndWatchServer) error {
	logFmt := "devicePluginServer(ListAndWatch): %v"

	resp := &pluginapi.ListAndWatchResponse{
		Devices: make([]*pluginapi.Device, resourceCount),
	}
	for i := 0; i < resourceCount; i++ {
		resp.Devices[i] = &pluginapi.Device{
			ID:     fmt.Sprintf("%s/%v", s.resourceName, i),
			Health: pluginapi.Healthy,
		}
	}

	go func() {
		for {
			select {
			case <-s.ctx.Done():
				logrus.Infof(logFmt, "server stopped")
				return
			case <-time.After(kubeletNotifyDelay):
				if err := server.Send(resp); err != nil {
					logrus.Errorf(logFmt, "server unavailable")
					return
				}
			}
		}
	}()

	return nil
}

func (s *devicePluginServer) Allocate(ctx context.Context, request *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	response := &pluginapi.AllocateResponse{
		ContainerResponses: make([]*pluginapi.ContainerAllocateResponse, len(request.ContainerRequests)),
	}

	for i := range request.ContainerRequests {
		hostPath := path.Join(hostBaseDir, uuid.New().String())
		response.ContainerResponses[i] = &pluginapi.ContainerAllocateResponse{
			Envs: map[string]string{
				hostPathEnv: hostPath,
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
