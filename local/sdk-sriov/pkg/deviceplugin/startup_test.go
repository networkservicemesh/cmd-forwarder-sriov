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
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var devicePluginPath = path.Join(os.TempDir(), "device-plugins")
var kubeSocket = path.Join(devicePluginPath, kubeletSocket)

func TestDevicePluginManager_MonitorKubeletRestart(t *testing.T) {
	dpm := NewManager(devicePluginPath)

	_ = os.RemoveAll(devicePluginPath)
	err := os.Mkdir(devicePluginPath, os.ModeDir|os.ModePerm)
	assert.Nil(t, err)

	monitorCh, err := dpm.MonitorKubeletRestart(context.TODO())
	assert.Nil(t, err)

	_, err = os.Create(kubeSocket)
	assert.Nil(t, err)

	select {
	case _, ok := <-monitorCh:
		assert.True(t, ok)
	case <-time.After(10 * time.Second):
		assert.Fail(t, "no update received")
	}
}
