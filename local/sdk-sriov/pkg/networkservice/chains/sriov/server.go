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

	"google.golang.org/grpc"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/kernel"
	vfiomech "github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/vfio"
	"github.com/networkservicemesh/sdk-kernel/pkg/kernel/networkservice/connectioncontext"
	"github.com/networkservicemesh/sdk-kernel/pkg/kernel/networkservice/inject"
	"github.com/networkservicemesh/sdk-kernel/pkg/kernel/networkservice/netns"
	"github.com/networkservicemesh/sdk-kernel/pkg/kernel/networkservice/rename"
	"github.com/networkservicemesh/sdk-sriov/pkg/networkservice/common/resourcepool"
	"github.com/networkservicemesh/sdk-sriov/pkg/networkservice/common/vfconfig"
	"github.com/networkservicemesh/sdk-sriov/pkg/networkservice/mechanisms/vfio"
	"github.com/networkservicemesh/sdk-sriov/pkg/sriov"
	pci "github.com/networkservicemesh/sdk-sriov/pkg/sriov/resourcepool"
	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/client"
	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/endpoint"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/clienturl"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/connect"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/mechanisms"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/mechanisms/recvfd"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/adapters"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/chain"
	"github.com/networkservicemesh/sdk/pkg/tools/addressof"
	"github.com/networkservicemesh/sdk/pkg/tools/token"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/networkservice/common/resetmechanism"
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
func NewServer(
	ctx context.Context,
	name string,
	authzServer networkservice.NetworkServiceServer,
	tokenGenerator token.GeneratorFunc,
	pciConfig *pci.Config,
	functions map[sriov.PCIFunction][]sriov.PCIFunction,
	binders map[uint][]sriov.DriverBinder,
	vfioDir, cgroupBaseDir string,
	clientURL *url.URL,
	clientDialOptions ...grpc.DialOption,
) endpoint.Endpoint {
	rv := sriovServer{}

	rv.Endpoint = endpoint.NewServer(
		ctx,
		name,
		authzServer,
		tokenGenerator,
		recvfd.NewServer(),
		vfconfig.NewServer(),
		resourcepool.NewInitServer(functions, pciConfig),
		resetmechanism.NewServer(
			mechanisms.NewServer(map[string]networkservice.NetworkServiceServer{
				kernel.MECHANISM: chain.NewNetworkServiceServer(
					resourcepool.NewServer(sriov.KernelDriver, functions, binders),
					rename.NewServer(),
					inject.NewServer(),
				),
				vfiomech.MECHANISM: chain.NewNetworkServiceServer(
					resourcepool.NewServer(sriov.VfioPCIDriver, functions, binders),
					vfio.NewServer(vfioDir, cgroupBaseDir),
				),
			}),
		),
		clienturl.NewServer(clientURL),
		connect.NewServer(ctx, client.NewClientFactory(
			name,
			// What to call onHeal
			addressof.NetworkServiceClient(adapters.NewServerToClient(rv)),
			tokenGenerator),
			clientDialOptions...,
		),
		netns.NewServer(),
		rename.NewServer(),
		connectioncontext.NewServer(),
	)

	return rv
}
