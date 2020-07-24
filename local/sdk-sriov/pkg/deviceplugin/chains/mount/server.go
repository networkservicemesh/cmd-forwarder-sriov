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

// Package mount provides chain element providing host -> client mount
package mount

import (
	"context"
	"path"

	"github.com/google/uuid"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/deviceplugin/chains"
)

type mountServer struct {
	hostBaseDir    string
	clientBaseDir  string
	hostBaseDirEnv string

	chains.EmptyDevicePluginServer
}

// NewServer creates a new chain element providing host -> client mount
func NewServer(hostBaseDir, clientBaseDir, hostBaseDirEnv string) chains.DevicePluginServer {
	return &mountServer{
		hostBaseDir:    hostBaseDir,
		clientBaseDir:  clientBaseDir,
		hostBaseDirEnv: hostBaseDirEnv,
	}
}

func (ms *mountServer) Allocate(ctx context.Context, request *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	response := &pluginapi.AllocateResponse{
		ContainerResponses: make([]*pluginapi.ContainerAllocateResponse, len(request.ContainerRequests)),
	}

	for i := range request.ContainerRequests {
		hostPath := path.Join(ms.hostBaseDir, uuid.New().String())
		response.ContainerResponses[i] = &pluginapi.ContainerAllocateResponse{
			Envs: map[string]string{
				ms.hostBaseDirEnv: hostPath,
			},
			Mounts: []*pluginapi.Mount{{
				ContainerPath: ms.clientBaseDir,
				HostPath:      hostPath,
				ReadOnly:      false,
			}},
		}
	}

	return response, nil
}
