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
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	podresources "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"

	testingtools "github.com/networkservicemesh/cmd-forwarder-sriov/test/tools"
)

const (
	resourceName    = "resource"
	resourceCount   = 10
	hostBaseDir     = "/base/dir"
	hostPathEnv     = "HOST_PATH"
	socket          = "socket"
	containersCount = 5
)

var config = &ServerConfig{
	ResourceName:  resourceName,
	ResourceCount: resourceCount,
	HostBaseDir:   hostBaseDir,
	HostPathEnv:   hostPathEnv,
}

func TestDevicePluginServer_Start(t *testing.T) {
	m := &managerMock{}
	m.mock.On("StartDeviceServer", mock.Anything, mock.Anything).
		Return(socket, nil)
	m.mock.On("RegisterDeviceServer", mock.Anything, mock.Anything).
		Return(nil)

	resetCh := make(chan bool)
	m.mock.On("MonitorKubeletRestart", mock.Anything).
		Return(resetCh, nil)

	m.mock.On("GetPodResourcesListerClient", mock.Anything).
		Return(struct {
			podresources.PodResourcesListerClient
		}{}, nil)

	err := StartServer(context.TODO(), config, m)
	assert.Nil(t, err)

	m.mock.AssertCalled(t, "StartDeviceServer", mock.Anything, mock.Anything)
	m.mock.AssertCalled(t, "RegisterDeviceServer", mock.Anything, mock.MatchedBy(func(request *pluginapi.RegisterRequest) bool {
		return request.Endpoint == socket && request.ResourceName == resourceNamePrefix+resourceName
	}))

	testingtools.WriteBoolChan(t, resetCh, true, 10*time.Second)
	assertNumberOfCallsEventually(t, m, "RegisterDeviceServer", 2)
}

func assertNumberOfCallsEventually(t *testing.T, m *managerMock, methodName string, expectedCalls int) {
	assert.Eventually(t, func() bool {
		var count int
		for i := range m.mock.Calls {
			if m.mock.Calls[i].Method == methodName {
				count++
			}
		}
		return count == expectedCalls
	}, 10*time.Second, 10*time.Millisecond)
}

func TestDevicePluginServer_ListAndWatch(t *testing.T) {
	resp := &podresources.ListPodResourcesResponse{}
	*resp = *listPodResourcesResponse([]string{})

	prlc := &podResourcesListerClientMock{}
	prlc.mock.On("List", mock.Anything, mock.Anything, mock.Anything).
		Return(resp, nil)

	dps := newDevicePluginServer(context.TODO(), config)
	dps.resourcePollTimeout = 10 * time.Millisecond
	dps.resourceListerClient = prlc

	respCh := make(chan *pluginapi.ListAndWatchResponse)
	lws := &listAndWatchServer{
		respCh: respCh,
	}

	go func() { _ = dps.ListAndWatch(&pluginapi.Empty{}, lws) }()

	// 1. Init device list

	validateResponse(t, respCh, resourceCount)

	// 2. Use some devices

	*resp = *listPodResourcesResponse(devices(0, 5))
	validateResponse(t, respCh, resourceCount+5)

	*resp = *listPodResourcesResponse(devices(0, 10))
	validateResponse(t, respCh, resourceCount+10)

	areCleanedUp := make([]bool, 10)
	for i, id := range devices(0, 10) {
		j := i
		dps.devices[id].clientCleanup = func() { areCleanedUp[j] = true }
	}

	// 3. Free some devices
	*resp = *listPodResourcesResponse(devices(5, 10))
	validateResponse(t, respCh, resourceCount+5)

	for i := 0; i < 10; i++ {
		assert.True(t, areCleanedUp[i] == (i < 5))
	}

	*resp = *listPodResourcesResponse([]string{})
	validateResponse(t, respCh, resourceCount)

	for i := 5; i < 10; i++ {
		assert.True(t, areCleanedUp[i])
	}
}

func listPodResourcesResponse(ids []string) *podresources.ListPodResourcesResponse {
	sort.Strings(ids)
	resp := &podresources.ListPodResourcesResponse{
		PodResources: []*podresources.PodResources{
			{},
		},
	}

	if len(ids) == 0 {
		return resp
	}

	step := len(ids)/3 + 1
	for i := 0; i < len(ids); i += step {
		devices := []*podresources.ContainerDevices{
			{
				ResourceName: resourceNamePrefix + resourceName,
			},
			{
				ResourceName: "wrong",
			},
		}
		for k := i; k < i+step && k < len(ids); k++ {
			devices[0].DeviceIds = append(devices[0].DeviceIds, ids[k])
			devices[1].DeviceIds = append(devices[1].DeviceIds, "wrong-"+ids[k])
		}
		resp.PodResources[0].Containers = append(resp.PodResources[0].Containers, &podresources.ContainerResources{
			Devices: devices,
		})
	}

	return resp
}

func validateResponse(t *testing.T, respCh chan *pluginapi.ListAndWatchResponse, expectedCount int) {
	resp, ok := testingtools.ReadListAndWatchResponseChan(t, respCh, 5*time.Minute)
	assert.True(t, ok)
	assert.Equal(t, expectedCount, len(resp.Devices))
	for _, device := range resp.Devices {
		assert.Equal(t, pluginapi.Healthy, device.Health)
	}
}

func devices(from, to int) []string {
	var devices []string
	for i := from; i < to; i++ {
		devices = append(devices, fmt.Sprintf("%s/%v", resourceNamePrefix+resourceName, i))
	}
	return devices
}

func TestDevicePluginServer_Allocate(t *testing.T) {
	dps := newDevicePluginServer(context.TODO(), config)

	areCleanedUp := make([]bool, containersCount)
	for i, id := range devices(0, containersCount) {
		j := i
		dps.devices[id] = &device{
			state:         deviceInUse,
			clientCleanup: func() { areCleanedUp[j] = true },
		}
	}

	req := &pluginapi.AllocateRequest{}
	for i := 0; i < containersCount; i++ {
		req.ContainerRequests = append(req.ContainerRequests, &pluginapi.ContainerAllocateRequest{
			DevicesIDs: devices(i, i+1),
		})
	}

	response, err := dps.Allocate(context.TODO(), req)
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

	for _, isCleanedUp := range areCleanedUp {
		assert.True(t, isCleanedUp)
	}
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

type managerMock struct {
	mock mock.Mock
}

func (m *managerMock) StartDeviceServer(ctx context.Context, deviceServer pluginapi.DevicePluginServer) (string, error) {
	res := m.mock.Called(ctx, deviceServer)
	return res.String(0), res.Error(1)
}

func (m *managerMock) RegisterDeviceServer(ctx context.Context, request *pluginapi.RegisterRequest) error {
	res := m.mock.Called(ctx, request)
	return res.Error(0)
}

func (m *managerMock) MonitorKubeletRestart(ctx context.Context) (chan bool, error) {
	res := m.mock.Called(ctx)
	return res.Get(0).(chan bool), res.Error(1)
}

func (m *managerMock) GetPodResourcesListerClient(ctx context.Context) (podresources.PodResourcesListerClient, error) {
	res := m.mock.Called(ctx)
	return res.Get(0).(podresources.PodResourcesListerClient), res.Error(1)
}

type podResourcesListerClientMock struct {
	mock mock.Mock
}

func (prlc *podResourcesListerClientMock) List(ctx context.Context, in *podresources.ListPodResourcesRequest, opts ...grpc.CallOption) (*podresources.ListPodResourcesResponse, error) {
	res := prlc.mock.Called(ctx, in, opts)
	return res.Get(0).(*podresources.ListPodResourcesResponse), res.Error(1)
}

type listAndWatchServer struct {
	respCh     chan<- *pluginapi.ListAndWatchResponse
	oldDevices []string

	pluginapi.DevicePlugin_ListAndWatchServer
}

func (lws *listAndWatchServer) Send(response *pluginapi.ListAndWatchResponse) error {
	var newDevices []string
	for _, device := range response.Devices {
		newDevices = append(newDevices, device.ID)
	}

	sort.Strings(newDevices)
	if !reflect.DeepEqual(newDevices, lws.oldDevices) {
		lws.oldDevices = newDevices
		lws.respCh <- response
	}

	return nil
}
