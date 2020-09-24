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

package k8s_test

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/k8s"
	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/tools/chan"
)

const (
	kubeletSocket = "kubelet.sock"
)

func TestDevicePluginManager_MonitorKubeletRestart(t *testing.T) {
	devicePluginPath := path.Join(os.TempDir(), t.Name())
	devicePluginSocket := path.Join(devicePluginPath, kubeletSocket)

	dpm := k8s.NewManager(devicePluginPath, "")

	_ = os.RemoveAll(devicePluginPath)
	err := os.MkdirAll(devicePluginPath, os.ModeDir|os.ModePerm)
	assert.Nil(t, err)

	monitorCh, err := dpm.MonitorKubeletRestart(context.TODO())
	assert.Nil(t, err)

	_, err = os.Create(devicePluginSocket)
	assert.Nil(t, err)

	_, ok := _chan.ReadBoolChan(t, monitorCh, 10*time.Second)
	assert.True(t, ok)
}
