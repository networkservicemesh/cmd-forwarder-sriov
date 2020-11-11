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

// Package sriov provides an Endpoint that implements the networks service for use as a Forwarder SRIOV
package sriov

import (
	"context"
	"net/url"

	"github.com/networkservicemesh/api/pkg/api/networkservice"

	"google.golang.org/grpc"

	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/client"
	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/endpoint"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/clienturl"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/connect"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/adapters"
	"github.com/networkservicemesh/sdk/pkg/tools/addressof"
	"github.com/networkservicemesh/sdk/pkg/tools/token"
)

type sriovServer struct {
	endpoint.Endpoint
}

// NewServer - returns a nsm client for use as a Forwarder
//             -name - name of the Forwarder
//             -authzServer - policy for allowing or rejecting requests
//             -tokenGenerator - token.GeneratorFunc - generates tokens for use in Path
//             -clientUrl - *url.URL for the talking to the NSMgr
//             -...clientDialOptions - dialOptions for dialing the NSMgr
func NewServer(ctx context.Context, name string, authzServer networkservice.NetworkServiceServer, tokenGenerator token.GeneratorFunc, clientURL *url.URL, clientDialOptions ...grpc.DialOption) endpoint.Endpoint {
	rv := sriovServer{}
	rv.Endpoint = endpoint.NewServer(
		ctx,
		name,
		authzServer,
		tokenGenerator,
		clienturl.NewServer(clientURL),
		connect.NewServer(
			ctx,
			client.NewClientFactory(
				name,
				// What to call onHeal
				addressof.NetworkServiceClient(adapters.NewServerToClient(rv)),
				tokenGenerator),
			clientDialOptions...,
		),
	)
	return rv
}
