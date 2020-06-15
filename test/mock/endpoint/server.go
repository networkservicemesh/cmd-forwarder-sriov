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

// Package endpoint define a test endpoint listening on passed URL.
package endpoint

import (
	"context"
	"errors"
	"math/rand"
	"net/url"
	"strings"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/config"
	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/types"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	"google.golang.org/grpc"
)

const (
	availablePortsKey string = "availablePorts"
	endpointPortKey   string = "endpointPort"

	configFileName string = "configPorts.json"
)

type nseImpl struct {
	server    *grpc.Server
	ctx       context.Context
	cancel    context.CancelFunc
	listenOn  *url.URL
	errorChan <-chan error
}

var configList *types.ResourceConfigList

// NewServer a new endpoint and running on grpc server
func NewServer(ctx context.Context, listenOn *url.URL) (server *grpc.Server, errChan <-chan error) {
	nse := &nseImpl{
		listenOn: listenOn,
		server:   grpc.NewServer(),
	}
	networkservice.RegisterNetworkServiceServer(nse.server, nse)

	nse.ctx, nse.cancel = context.WithCancel(ctx)
	nse.errorChan = grpcutils.ListenAndServe(nse.ctx, nse.listenOn, nse.server)

	configList, _ = config.ReadConfig(configFileName)

	return nse.server, nse.errorChan
}

func (d *nseImpl) Request(ctx context.Context, request *networkservice.NetworkServiceRequest) (*networkservice.Connection, error) {
	request.Connection.Mechanism.Parameters = map[string]string{}
	err := errors.New("'availablePorts' is empty")

	if macListStr, exist := request.MechanismPreferences[0].Parameters[availablePortsKey]; exist {
		macList := strings.Split(macListStr, ",")
		err = nil

		// select from received ports
		if configList == nil {
			randomIndex := rand.Intn(len(macList))
			request.Connection.Mechanism.Parameters = map[string]string{endpointPortKey: macList[randomIndex]}
		} else { // search supported port
			for _, config := range configList.ResourceList {
				for _, mac := range macList {
					if config.ConnectedToPort == mac {
						request.Connection.Mechanism.Parameters = map[string]string{endpointPortKey: mac}

						return request.GetConnection(), nil
					}
				}
			}

			err = errors.New("Specified ports are not supported")
		}
	}

	return request.GetConnection(), err
}

func (d *nseImpl) Close(ctx context.Context, connection *networkservice.Connection) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}
