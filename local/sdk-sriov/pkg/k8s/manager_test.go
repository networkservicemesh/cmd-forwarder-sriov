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

package k8s

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	podresources "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"

	testingtools "github.com/networkservicemesh/cmd-forwarder-sriov/test/tools"
)

var devicePluginPath = path.Join(os.TempDir(), "device-plugins")
var devicePluginSocket = path.Join(devicePluginPath, kubeletSocket)

func TestDevicePluginManager_MonitorKubeletRestart(t *testing.T) {
	dpm := NewManager(devicePluginPath, "")

	_ = os.RemoveAll(devicePluginPath)
	err := os.Mkdir(devicePluginPath, os.ModeDir|os.ModePerm)
	assert.Nil(t, err)

	monitorCh, err := dpm.MonitorKubeletRestart(context.TODO())
	assert.Nil(t, err)

	_, err = os.Create(devicePluginSocket)
	assert.Nil(t, err)

	_, ok := testingtools.ReadBoolChan(t, monitorCh, 10*time.Second)
	assert.True(t, ok)
}

type ManagerMock struct {
	Mock mock.Mock
}

func (m *ManagerMock) StartDeviceServer(ctx context.Context, deviceServer pluginapi.DevicePluginServer) (string, error) {
	res := m.Mock.Called(ctx, deviceServer)
	return res.String(0), res.Error(1)
}

func (m *ManagerMock) RegisterDeviceServer(ctx context.Context, request *pluginapi.RegisterRequest) error {
	res := m.Mock.Called(ctx, request)
	return res.Error(0)
}

func (m *ManagerMock) MonitorKubeletRestart(ctx context.Context) (chan bool, error) {
	res := m.Mock.Called(ctx)
	return res.Get(0).(chan bool), res.Error(1)
}

func (m *ManagerMock) GetPodResourcesListerClient(ctx context.Context) (podresources.PodResourcesListerClient, error) {
	res := m.Mock.Called(ctx)
	return res.Get(0).(podresources.PodResourcesListerClient), res.Error(1)
}
