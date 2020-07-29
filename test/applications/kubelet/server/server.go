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

// Package kubelet provides kubelet stub application for device plugin testing without k8s
package kubelet

import (
	"context"
	"path"

	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/tools"
)

type registrationServer struct {
	devicePluginPath string
}

// NewRegistrationServer creates a new registrationServer and registers it on given GRPC server
func NewRegistrationServer(devicePluginPath string, server *grpc.Server) pluginapi.RegistrationServer {
	rs := &registrationServer{
		devicePluginPath: devicePluginPath,
	}
	pluginapi.RegisterRegistrationServer(server, rs)
	return rs
}

func (rs *registrationServer) Register(ctx context.Context, request *pluginapi.RegisterRequest) (*pluginapi.Empty, error) {
	socketPath := tools.SocketPath(path.Join(rs.devicePluginPath, request.Endpoint))
	socketURL := grpcutils.AddressToURL(socketPath)
	conn, err := grpc.DialContext(ctx, socketURL.String(), grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	client := pluginapi.NewDevicePluginClient(conn)
	receiver, err := client.ListAndWatch(ctx, &pluginapi.Empty{})
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			response, err := receiver.Recv()
			if err != nil {
				return
			}
			logrus.Infof("registrationServer(Register): devices update -> %v", response.Devices)
		}
	}()

	return &pluginapi.Empty{}, nil
}
