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

// Package deviceplugin provides tools to setup device plugin server
package deviceplugin

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/tools"
)

const dialTimeoutDefault = 15 * time.Second

// StartDeviceServer starts device plugin server and returns the name of the corresponding unix socket
func StartDeviceServer(ctx context.Context, deviceServer pluginapi.DevicePluginServer) (string, error) {
	logFmt := "StartDeviceServer: %v"

	socket := uuid.New().String()
	socketPath := tools.SocketPath(path.Join(pluginapi.DevicePluginPath, socket))
	logrus.Infof(logFmt, fmt.Sprint("socket = ", socket))
	if err := tools.SocketCleanup(socketPath); err != nil {
		return "", err
	}

	grpcServer := grpc.NewServer()
	pluginapi.RegisterDevicePluginServer(grpcServer, deviceServer)
	errCh := grpcutils.ListenAndServe(ctx, grpcutils.AddressToURL(socketPath), grpcServer)
	go func() {
		if err := <-errCh; err != nil {
			logrus.Errorf(logFmt, fmt.Sprint("failed to start device plugin grpc server ", socketPath, err))
		}
	}()

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeoutDefault)
	defer cancel()

	logrus.Infof(logFmt, "check device server operational")
	conn, err := grpc.DialContext(dialCtx, socketPath.String(), grpc.WithBlock())
	if err != nil {
		logrus.Errorf(logFmt, err)
		return "", err
	}
	_ = conn.Close()

	logrus.Infof(logFmt, "device server is operational")

	return socket, nil
}

// RegisterDeviceServer registers device plugin server using the given request
func RegisterDeviceServer(ctx context.Context, request *pluginapi.RegisterRequest) error {
	logFmt := "RegisterDeviceServer: %v"

	socketPath := tools.SocketPath(path.Join(pluginapi.DevicePluginPath, request.Endpoint))
	conn, err := grpc.DialContext(ctx, socketPath.String())
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf(logFmt, "cannot connect to kubelet service"))
	}
	defer func() { _ = conn.Close() }()

	client := pluginapi.NewRegistrationClient(conn)
	logrus.Infof(logFmt, "trying to register to kubelet service")
	if _, err = client.Register(context.Background(), request); err != nil {
		return errors.Wrap(err, fmt.Sprintf(logFmt, "cannot register to kubelet service"))
	}
	logrus.Infof(logFmt, "register done")

	return nil
}
