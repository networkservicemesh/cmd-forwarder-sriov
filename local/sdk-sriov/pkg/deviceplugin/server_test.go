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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	resourceName    = "resource"
	resourceCount   = 10
	hostBaseDir     = "/base/dir"
	hostPathEnv     = "HOST_PATH"
	socket          = "socket"
	containersCount = 5
)

func TestDevicePluginServer_Start(t *testing.T) {
	dps := NewServer(resourceName, resourceCount, hostBaseDir, hostPathEnv)

	m := &manager{}
	m.On("StartDeviceServer", mock.Anything, mock.Anything).Return(socket, nil)
	m.On("RegisterDeviceServer", mock.Anything, mock.Anything).Return(nil)

	err := dps.Start(context.TODO(), m)
	assert.Nil(t, err)

	m.AssertCalled(t, "StartDeviceServer", mock.Anything, dps)
	m.AssertCalled(t, "RegisterDeviceServer", mock.Anything, mock.MatchedBy(func(request *pluginapi.RegisterRequest) bool {
		return request.Endpoint == socket && request.ResourceName == resourceNamePrefix+resourceName
	}))
}

func TestDevicePluginServer_ListAndWatch(t *testing.T) {
	dps := NewServer(resourceName, resourceCount, hostBaseDir, hostPathEnv).(*devicePluginServer)
	dps.ctx = context.TODO()

	respCh := make(chan *pluginapi.ListAndWatchResponse)
	lws := &listAndWatchServer{
		respCh: respCh,
	}

	go func() { _ = dps.ListAndWatch(&pluginapi.Empty{}, lws) }()

	select {
	case resp, ok := <-respCh:
		assert.True(t, ok)
		assert.Condition(t, func() bool {
			if len(resp.Devices) != resourceCount {
				return false
			}
			for _, device := range resp.Devices {
				if device.Health != pluginapi.Healthy {
					return false
				}
			}
			return true
		})
	case <-time.After(kubeletNotifyDelay * 2):
		assert.Fail(t, "no response received")
	}
}

func TestDevicePluginServer_Allocate(t *testing.T) {
	dps := NewServer(resourceName, resourceCount, hostBaseDir, hostPathEnv).(*devicePluginServer)

	response, err := dps.Allocate(context.TODO(), &pluginapi.AllocateRequest{
		ContainerRequests: make([]*pluginapi.ContainerAllocateRequest, containersCount),
	})
	assert.Nil(t, err)
	assert.Condition(t, func() bool {
		if len(response.ContainerResponses) != containersCount {
			return false
		}
		for _, container := range response.ContainerResponses {
			if !testContainerResponse(container) {
				return false
			}
		}
		return true
	})
}

func testContainerResponse(container *pluginapi.ContainerAllocateResponse) bool {
	if hostPath, ok := container.Envs[hostPathEnv]; ok {
		for _, mount := range container.Mounts {
			if mount.ContainerPath == vfioDir && mount.HostPath == hostPath && !mount.ReadOnly {
				return true
			}
		}
	}
	return false
}

type manager struct {
	mock.Mock

	Manager
}

func (m *manager) StartDeviceServer(ctx context.Context, deviceServer pluginapi.DevicePluginServer) (string, error) {
	res := m.Called(ctx, deviceServer)
	return res.String(0), res.Error(1)
}

func (m *manager) RegisterDeviceServer(ctx context.Context, request *pluginapi.RegisterRequest) error {
	res := m.Called(ctx, request)
	return res.Error(0)
}

type listAndWatchServer struct {
	respCh chan<- *pluginapi.ListAndWatchResponse

	pluginapi.DevicePlugin_ListAndWatchServer
}

func (lws *listAndWatchServer) Send(response *pluginapi.ListAndWatchResponse) error {
	lws.respCh <- response
	return nil
}
