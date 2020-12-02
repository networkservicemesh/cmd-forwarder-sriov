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

//+build !windows

// Package sriovns provides an Endpoint implementing the SR-IOV Forwarder networks service
package sriovns

import (
	"context"
	"net/url"
	"sync"

	sriovconfig "github.com/networkservicemesh/sdk-sriov/pkg/sriov/config"
	"google.golang.org/grpc"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/cls"
	"github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/kernel"
	noopmech "github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/noop"
	vfiomech "github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/vfio"
	"github.com/networkservicemesh/sdk-kernel/pkg/kernel/networkservice/ethernetcontext"
	"github.com/networkservicemesh/sdk-kernel/pkg/kernel/networkservice/inject"
	"github.com/networkservicemesh/sdk-kernel/pkg/kernel/networkservice/ipcontext"
	"github.com/networkservicemesh/sdk-kernel/pkg/kernel/networkservice/netns"
	"github.com/networkservicemesh/sdk-kernel/pkg/kernel/networkservice/rename"
	"github.com/networkservicemesh/sdk-sriov/pkg/networkservice/common/mechanisms/vfio"
	"github.com/networkservicemesh/sdk-sriov/pkg/networkservice/common/resourcepool"
	"github.com/networkservicemesh/sdk-sriov/pkg/networkservice/common/vfconfig"
	"github.com/networkservicemesh/sdk-sriov/pkg/sriov"
	"github.com/networkservicemesh/sdk-sriov/pkg/sriov/resource"
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

	"github.com/networkservicemesh/cmd-forwarder-sriov/internal/networkservice/common/noop"
	"github.com/networkservicemesh/cmd-forwarder-sriov/internal/networkservice/common/resetmechanism"
)

type sriovServer struct {
	endpoint.Endpoint
}

// NewServer - returns an Endpoint implementing the SR-IOV Forwarder networks service
//             - name - name of the Forwarder
//             - authzServer - policy for allowing or rejecting requests
//             - tokenGenerator - token.GeneratorFunc - generates tokens for use in Path
//             - tokenPool - provides SR-IOV resource tokens
//             - sriovConfig - SR-IOV PCI functions config
//             - functions - SR-IOV PCI functions (PF -> []VF)
//             - binders - SR-IOV PCI driver binders (IOMMU group -> []PCI Function driver binder)
//             - vfioDir - host /dev/vfio directory mount location
//             - cgroupBaseDir - host /sys/fs/cgroup/devices directory mount location
//             - clientUrl - *url.URL for the talking to the NSMgr
//             - ...clientDialOptions - dialOptions for dialing the NSMgr
func NewServer(
	ctx context.Context,
	name string,
	authzServer networkservice.NetworkServiceServer,
	tokenGenerator token.GeneratorFunc,
	tokenPool resource.TokenPool,
	sriovConfig *sriovconfig.Config,
	functions map[sriov.PCIFunction][]sriov.PCIFunction,
	binders map[uint][]sriov.DriverBinder,
	vfioDir, cgroupBaseDir string,
	clientURL *url.URL,
	clientDialOptions ...grpc.DialOption,
) endpoint.Endpoint {
	rv := sriovServer{}

	connectChainFactory := func(class string) networkservice.NetworkServiceServer {
		return chain.NewNetworkServiceServer(
			clienturl.NewServer(clientURL),
			connect.NewServer(ctx,
				client.NewCrossConnectClientFactory(
					name,
					// What to call onHeal
					addressof.NetworkServiceClient(adapters.NewServerToClient(rv)),
					tokenGenerator,
					noop.NewClient(class),
				),
				clientDialOptions...,
			),
		)
	}

	sriovChain := sriovChain(
		tokenPool,
		sriovConfig,
		functions,
		binders,
		vfioDir, cgroupBaseDir,
		connectChainFactory,
	)

	rv.Endpoint = endpoint.NewServer(
		ctx,
		name,
		authzServer,
		tokenGenerator,
		mechanisms.NewServer(map[string]networkservice.NetworkServiceServer{
			kernel.MECHANISM:   sriovChain,
			vfiomech.MECHANISM: sriovChain,
			noopmech.MECHANISM: connectChainFactory(cls.LOCAL),
		}),
	)

	return rv
}

func sriovChain(
	tokenPool resource.TokenPool,
	sriovConfig *sriovconfig.Config,
	functions map[sriov.PCIFunction][]sriov.PCIFunction,
	binders map[uint][]sriov.DriverBinder,
	vfioDir, cgroupBaseDir string,
	connectChainFactory func(string) networkservice.NetworkServiceServer,
) networkservice.NetworkServiceServer {
	resourceLock := &sync.Mutex{}
	return chain.NewNetworkServiceServer(
		recvfd.NewServer(),
		vfconfig.NewServer(),
		resourcepool.NewInitServer(tokenPool, sriovConfig),
		resetmechanism.NewServer(
			mechanisms.NewServer(map[string]networkservice.NetworkServiceServer{
				kernel.MECHANISM: chain.NewNetworkServiceServer(
					resourcepool.NewServer(sriov.KernelDriver, resourceLock, functions, binders),
					rename.NewServer(),
					inject.NewServer(),
				),
				vfiomech.MECHANISM: chain.NewNetworkServiceServer(
					resourcepool.NewServer(sriov.VFIOPCIDriver, resourceLock, functions, binders),
					vfio.NewServer(vfioDir, cgroupBaseDir),
				),
			}),
		),
		connectChainFactory(cls.REMOTE),
		// we setup VF ethernet context using PF interface, so we do it in the forwarder net NS
		ethernetcontext.NewVFServer(),
		// now setup VF interface, so we do it in the client net NS
		netns.NewServer(),
		rename.NewServer(),
		ipcontext.NewServer(),
	)
}
