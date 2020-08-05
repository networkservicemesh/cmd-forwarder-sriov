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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	podresources "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"

	testingtools "github.com/networkservicemesh/cmd-forwarder-sriov/test/tools"
)

var testPodResources = [][]*podresources.PodResources{
	{
		{
			Containers: []*podresources.ContainerResources{
				{
					Devices: []*podresources.ContainerDevices{
						{
							ResourceName: resourceName,
							DeviceIds:    devicesIds("a", 0, 4),
						},
						{
							ResourceName: "wrong",
							DeviceIds:    devicesIds("b", 0, 4),
						},
					},
				},
				{
					Devices: []*podresources.ContainerDevices{
						{
							ResourceName: resourceName,
							DeviceIds:    devicesIds("a", 4, 8),
						},
						{
							ResourceName: "wrong",
							DeviceIds:    devicesIds("b", 4, 8),
						},
					},
				},
			},
		},
	},
	{
		{
			Containers: []*podresources.ContainerResources{
				{
					Devices: []*podresources.ContainerDevices{
						{
							ResourceName: resourceName,
							DeviceIds:    devicesIds("a", 4, 8),
						},
						{
							ResourceName: "wrong",
							DeviceIds:    devicesIds("b", 4, 8),
						},
					},
				},
			},
		},
	},
}

func TestDeviceMonitor_Monitor(t *testing.T) {
	m := &managerMock{}
	prlc := &podResourcesListerClientStub{}
	m.mock.On("GetPodResourcesListerClient", mock.Anything).
		Return(prlc, nil)

	dm, err := newDeviceMonitor(context.TODO(), m)
	assert.Nil(t, err)

	devicesCh, _ := dm.Monitor(resourceName)

	prlc.podResources = testPodResources[0]
	devices, ok := testingtools.ReadStringArrChan(t, devicesCh, 2*pollingTimeout)
	assert.True(t, ok)
	assert.Equal(t, devicesIds("a", 0, 8), devices)

	prlc.podResources = testPodResources[1]
	devices, ok = testingtools.ReadStringArrChan(t, devicesCh, 2*pollingTimeout)
	assert.True(t, ok)
	assert.Equal(t, devicesIds("a", 4, 8), devices)

	prlc.podResources = []*podresources.PodResources{}
	devices, ok = testingtools.ReadStringArrChan(t, devicesCh, 2*pollingTimeout)
	assert.True(t, ok)
	assert.Empty(t, devices)
}

func devicesIds(prefix string, from, to int) []string {
	var res []string
	for i := from; i < to; i++ {
		res = append(res, fmt.Sprint(prefix, i))
	}
	return res
}

type podResourcesListerClientStub struct {
	podResources []*podresources.PodResources
}

func (prlc *podResourcesListerClientStub) List(_ context.Context, _ *podresources.ListPodResourcesRequest, _ ...grpc.CallOption) (*podresources.ListPodResourcesResponse, error) {
	return &podresources.ListPodResourcesResponse{
		PodResources: prlc.podResources,
	}, nil
}
