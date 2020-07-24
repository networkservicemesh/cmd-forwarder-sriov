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

// Package resource provides chain element providing k8s resource
package resource

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/deviceplugin/chains"
)

const (
	kubeletNotifyDelay = 30 * time.Second
)

type resourceServer struct {
	ctx           context.Context
	resourceName  string
	resourceCount int

	chains.EmptyDevicePluginServer
}

// NewServer creates a new chain element providing k8s resource
func NewServer(ctx context.Context, resourceName string, resourceCount int) chains.DevicePluginResourceServer {
	return &resourceServer{
		ctx:           ctx,
		resourceName:  resourceName,
		resourceCount: resourceCount,
	}
}

func (rs *resourceServer) ResourceName() string {
	return rs.resourceName
}

func (rs *resourceServer) ListAndWatch(receiver chan<- *pluginapi.ListAndWatchResponse) error {
	resp := &pluginapi.ListAndWatchResponse{
		Devices: make([]*pluginapi.Device, rs.resourceCount),
	}
	for i := 0; i < rs.resourceCount; i++ {
		resp.Devices[i] = &pluginapi.Device{
			ID:     fmt.Sprintf("%s/%v", rs.resourceName, i),
			Health: pluginapi.Healthy,
		}
	}

	for {
		select {
		case <-rs.ctx.Done():
			logrus.Info("ResourceServer(ListAndWatch): server stopped")
			return nil
		case <-time.After(kubeletNotifyDelay):
			receiver <- resp
		}
	}
}

var _ chains.DevicePluginResourceServer = (*resourceServer)(nil)
