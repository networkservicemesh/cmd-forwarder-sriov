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

package chains

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

func TestServerChain_GetDevicePluginOptions_False(t *testing.T) {
	dpss := newDevicePluginServerStubs(2)

	sc := NewDevicePluginServerChain(dpss[0], dpss[1])

	options, err := sc.GetDevicePluginOptions(context.TODO(), &pluginapi.Empty{})
	assert.Nil(t, err)
	assert.False(t, options.PreStartRequired)
}

func TestServerChain_GetDevicePluginOptions_True(t *testing.T) {
	dpss := newDevicePluginServerStubs(2)
	dpss[1].preStartRequired = true

	sc := NewDevicePluginServerChain(dpss[0], dpss[1])

	options, err := sc.GetDevicePluginOptions(context.TODO(), &pluginapi.Empty{})
	assert.Nil(t, err)
	assert.True(t, options.PreStartRequired)
}

func TestServerChain_ListAndWatch(t *testing.T) {
	dpss := newDevicePluginServerStubs(3)

	sc := NewDevicePluginServerChain(dpss[0], dpss[1], dpss[2])

	respCh := make(chan *pluginapi.ListAndWatchResponse, 1)
	err := sc.ListAndWatch(&pluginapi.Empty{}, &listAndWatchServerStub{respCh: respCh})
	assert.Nil(t, err)

	go func() { dpss[0].updateCh <- true }()
	assert.Empty(t, (<-respCh).Devices)

	dpss[0].devices = createTestMap("a", "b")
	go func() { dpss[0].updateCh <- true }()
	assert.Equal(t, devicesToMap((<-respCh).Devices), createTestMap("a", "b"))

	testMap := createTestMap("a", "b", "c", "d")

	dpss[1].devices = createTestMap("c", "d")
	go func() { dpss[1].updateCh <- true }()
	assert.Equal(t, devicesToMap((<-respCh).Devices), testMap)

	dpss[1].devices["c"] = "d"
	dpss[1].devices["d"] = "e"
	testMap["c"] = "d"
	testMap["d"] = "e"
	go func() { dpss[1].updateCh <- true }()
	assert.Equal(t, devicesToMap((<-respCh).Devices), testMap)

	dpss[1].devices = map[string]string{}
	go func() { dpss[1].updateCh <- true }()
	assert.Equal(t, devicesToMap((<-respCh).Devices), createTestMap("a", "b"))
}

type listAndWatchServerStub struct {
	respCh chan *pluginapi.ListAndWatchResponse

	pluginapi.DevicePlugin_ListAndWatchServer
}

func (mlws *listAndWatchServerStub) Send(response *pluginapi.ListAndWatchResponse) error {
	mlws.respCh <- response
	return nil
}

func TestServerChain_Allocate_Different(t *testing.T) {
	dpss := newDevicePluginServerStubs(3)
	dpss[0].envs = createTestMap("a", "b")
	dpss[0].mounts = createTestMap("a", "b")
	dpss[0].deviceSpecs = createTestMap("a", "b")
	dpss[1].envs = createTestMap("c", "d")
	dpss[1].mounts = createTestMap("c", "d")
	dpss[1].deviceSpecs = createTestMap("c", "d")

	sc := NewDevicePluginServerChain(dpss[0], dpss[1], dpss[2])

	response, err := sc.Allocate(context.TODO(), &pluginapi.AllocateRequest{
		ContainerRequests: make([]*pluginapi.ContainerAllocateRequest, 1),
	})
	assert.Nil(t, err)
	assert.Equal(t, createTestMap("a", "b", "c", "d"), response.ContainerResponses[0].Envs)
	assert.Equal(t, createTestMap("a", "b", "c", "d"), mountsToMap(response.ContainerResponses[0].Mounts))
	assert.Equal(t, createTestMap("a", "b", "c", "d"), deviceSpecsToMap(response.ContainerResponses[0].Devices))
}

func TestServerChain_Allocate_Overlaying(t *testing.T) {
	dpss := newDevicePluginServerStubs(3)
	dpss[0].envs = createTestMap("a", "b", "c")
	dpss[0].mounts = createTestMap("a", "b", "c")
	dpss[0].deviceSpecs = createTestMap("a", "b", "c")
	dpss[1].envs = createTestMap("b", "c", "d")
	dpss[1].mounts = createTestMap("b", "c", "d")
	dpss[1].deviceSpecs = createTestMap("b", "c", "d")

	sc := NewDevicePluginServerChain(dpss[0], dpss[1], dpss[2])

	response, err := sc.Allocate(context.TODO(), &pluginapi.AllocateRequest{
		ContainerRequests: make([]*pluginapi.ContainerAllocateRequest, 1),
	})
	assert.Nil(t, err)
	assert.Equal(t, createTestMap("a", "b", "c", "d"), response.ContainerResponses[0].Envs)
	assert.Equal(t, createTestMap("a", "b", "c", "d"), mountsToMap(response.ContainerResponses[0].Mounts))
	assert.Equal(t, createTestMap("a", "b", "c", "d"), deviceSpecsToMap(response.ContainerResponses[0].Devices))
}

func createTestMap(keys ...string) map[string]string {
	res := map[string]string{}
	for _, k := range keys {
		res[k] = k
	}
	return res
}

type devicePluginServerStub struct {
	resourceName     string
	preStartRequired bool
	envs             map[string]string
	mounts           map[string]string
	deviceSpecs      map[string]string
	devices          map[string]string
	updateCh         chan bool

	EmptyDevicePluginServer
	mock.Mock
}

func newDevicePluginServerStubs(size int) []*devicePluginServerStub {
	var res []*devicePluginServerStub
	for i := 0; i < size; i++ {
		res = append(res, &devicePluginServerStub{
			updateCh: make(chan bool, 1),
		})
	}
	return res
}

func (dps *devicePluginServerStub) ResourceName() string {
	return dps.resourceName
}

func (dps *devicePluginServerStub) GetDevicePluginOptions(_ context.Context) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{
		PreStartRequired: dps.preStartRequired,
	}, nil
}

func (dps *devicePluginServerStub) ListAndWatch(receiver chan<- *pluginapi.ListAndWatchResponse) error {
	go func() {
		for <-dps.updateCh {
			receiver <- &pluginapi.ListAndWatchResponse{
				Devices: mapToDevices(dps.devices),
			}
		}
	}()
	return nil
}

func mapToDevices(m map[string]string) []*pluginapi.Device {
	var devices []*pluginapi.Device
	for id, health := range m {
		devices = append(devices, &pluginapi.Device{
			ID:     id,
			Health: health,
		})
	}
	return devices
}

func devicesToMap(devices []*pluginapi.Device) map[string]string {
	m := map[string]string{}
	for _, device := range devices {
		m[device.ID] = device.Health
	}
	return m
}

func (dps *devicePluginServerStub) Allocate(_ context.Context, request *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	response := &pluginapi.AllocateResponse{
		ContainerResponses: make([]*pluginapi.ContainerAllocateResponse, len(request.ContainerRequests)),
	}

	mounts := mapToMounts(dps.mounts)
	deviceSpecs := mapToDeviceSpecs(dps.deviceSpecs)

	for i := range request.ContainerRequests {
		response.ContainerResponses[i] = &pluginapi.ContainerAllocateResponse{
			Envs:    dps.envs,
			Mounts:  mounts,
			Devices: deviceSpecs,
		}
	}

	return response, nil
}

func mapToMounts(m map[string]string) []*pluginapi.Mount {
	var mounts []*pluginapi.Mount
	for host, client := range m {
		mounts = append(mounts, &pluginapi.Mount{
			ContainerPath: client,
			HostPath:      host,
		})
	}
	return mounts
}

func mountsToMap(mounts []*pluginapi.Mount) map[string]string {
	var m = map[string]string{}
	for _, mount := range mounts {
		m[mount.HostPath] = mount.ContainerPath
	}
	return m
}

func mapToDeviceSpecs(m map[string]string) []*pluginapi.DeviceSpec {
	var deviceSpecs []*pluginapi.DeviceSpec
	for host, client := range m {
		deviceSpecs = append(deviceSpecs, &pluginapi.DeviceSpec{
			ContainerPath: client,
			HostPath:      host,
		})
	}
	return deviceSpecs
}

func deviceSpecsToMap(deviceSpecs []*pluginapi.DeviceSpec) map[string]string {
	var m = map[string]string{}
	for _, deviceSpec := range deviceSpecs {
		m[deviceSpec.HostPath] = deviceSpec.ContainerPath
	}
	return m
}
