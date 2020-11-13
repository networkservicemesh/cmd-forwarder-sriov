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

package main

import (
	"context"
	"net/url"
	"os"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/edwarnicke/grpcfd"
	"github.com/golang/protobuf/ptypes"
	"github.com/kelseyhightower/envconfig"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	registryapi "github.com/networkservicemesh/api/pkg/api/registry"
	"github.com/networkservicemesh/sdk-sriov/pkg/sriov"
	sriovconfig "github.com/networkservicemesh/sdk-sriov/pkg/sriov/config"
	"github.com/networkservicemesh/sdk-sriov/pkg/sriov/pcifunction"
	"github.com/networkservicemesh/sdk-sriov/pkg/sriov/token"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/authorize"
	registrysendfd "github.com/networkservicemesh/sdk/pkg/registry/common/sendfd"
	registrychain "github.com/networkservicemesh/sdk/pkg/registry/core/chain"
	"github.com/networkservicemesh/sdk/pkg/tools/debug"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/signalctx"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"

	"github.com/networkservicemesh/cmd-forwarder-sriov/internal/k8s"
	"github.com/networkservicemesh/cmd-forwarder-sriov/internal/k8s/deviceplugin"
	"github.com/networkservicemesh/cmd-forwarder-sriov/internal/networkservice/chains/sriovns"
)

// Config - configuration for cmd-forwarder-sriov
type Config struct {
	Name                string        `default:"interpose-nse#sriov-forwarder" desc:"Name of Endpoint"`
	NSName              string        `default:"sriovns" desc:"Name of Network Service to Register with Registry"`
	ListenOn            url.URL       `default:"unix:///listen.on.socket" desc:"url to listen on" split_words:"true"`
	ConnectTo           url.URL       `default:"unix:///connect.to.socket" desc:"url to connect to" split_words:"true"`
	MaxTokenLifetime    time.Duration `default:"24h" desc:"maximum lifetime of tokens" split_words:"true"`
	ResourcePollTimeout time.Duration `default:"30s" desc:"device plugin polling timeout" split_words:"true"`
	DevicePluginPath    string        `default:"/var/lib/kubelet/device-plugins/" desc:"path to the device plugin directory" split_words:"true"`
	PodResourcesPath    string        `default:"/var/lib/kubelet/pod-resources/" desc:"path to the pod resources directory" split_words:"true"`
	SRIOVConfigFile     string        `default:"pci.config" desc:"PCI resources config path" split_words:"true"`
	PCIDevicesPath      string        `default:"/sys/bus/pci/devices" desc:"path to the PCI devices directory" split_words:"true"`
	PCIDriversPath      string        `default:"/sys/bus/pci/drivers" desc:"path to the PCI drivers directory" split_words:"true"`
	CgroupPath          string        `default:"/host/sys/fs/cgroup/devices" desc:"path to the host cgroup directory" split_words:"true"`
	VFIOPath            string        `default:"/host/dev/vfio" desc:"path to the host VFIO directory" split_words:"true"`
}

func main() {
	// ********************************************************************************
	// setup context to catch signals
	// ********************************************************************************
	ctx := signalctx.WithSignals(context.Background())
	ctx, cancel := context.WithCancel(ctx)

	// ********************************************************************************
	// setup logging
	// ********************************************************************************
	logrus.SetFormatter(&nested.Formatter{})
	logrus.SetLevel(logrus.TraceLevel)
	ctx = log.WithField(ctx, "cmd", os.Args[0])

	// ********************************************************************************
	// Debug self if necessary
	// ********************************************************************************
	if err := debug.Self(); err != nil {
		log.Entry(ctx).Infof("%s", err)
	}

	starttime := time.Now()

	// enumerating phases
	log.Entry(ctx).Infof("there are 7 phases which will be executed followed by a success message:")
	log.Entry(ctx).Infof("the phases include:")
	log.Entry(ctx).Infof("1: get config from environment")
	log.Entry(ctx).Infof("2: init PCI functions")
	log.Entry(ctx).Infof("3: start device plugin server")
	log.Entry(ctx).Infof("4: retrieve spiffe svid")
	log.Entry(ctx).Infof("5: create sriovns network service endpoint")
	log.Entry(ctx).Infof("6: create grpc server and register sriovns")
	log.Entry(ctx).Infof("7: register xconnectns with the registry")
	log.Entry(ctx).Infof("a final success message with start time duration")

	// ********************************************************************************
	log.Entry(ctx).Infof("executing phase 1: get config from environment (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	config := &Config{}
	if err := envconfig.Usage("nsm", config); err != nil {
		log.Entry(ctx).Fatal(err)
	}
	if err := envconfig.Process("nsm", config); err != nil {
		log.Entry(ctx).Fatalf("error processing config from env: %+v", err)
	}

	log.Entry(ctx).Infof("Config: %#v", config)

	// ********************************************************************************
	log.Entry(ctx).Infof("executing phase 2: init PCI functions (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	sriovConfig, functions, binders, err := initPCIFunctions(ctx, config.SRIOVConfigFile, config.PCIDevicesPath, config.PCIDriversPath)
	if err != nil {
		log.Entry(ctx).Fatalf("failed to configure PCI physical functions: %+v", err)
	}
	defer func() {
		for _, igBinders := range binders {
			for _, binder := range igBinders {
				_ = binder.BindKernelDriver()
			}
		}
	}()

	// ********************************************************************************
	log.Entry(ctx).Infof("executing phase 3: start device plugin server (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	// Create SR-IOV resource token pool
	tokenPool := token.NewPool(sriovConfig)

	// Start device plugin server
	manager := k8s.NewManager(config.DevicePluginPath, config.PodResourcesPath)
	if err = deviceplugin.StartServers(ctx, tokenPool, config.ResourcePollTimeout, manager); err != nil {
		log.Entry(ctx).Fatalf("failed to start a device plugin server: %+v", err)
	}

	// ********************************************************************************
	log.Entry(ctx).Infof("executing phase 4: retrieving svid, check spire agent logs if this is the last line you see (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	source, err := workloadapi.NewX509Source(ctx)
	if err != nil {
		log.Entry(ctx).Fatalf("error getting x509 source: %+v", err)
	}
	svid, err := source.GetX509SVID()
	if err != nil {
		log.Entry(ctx).Fatalf("error getting x509 svid: %+v", err)
	}
	log.Entry(ctx).Infof("SVID: %q", svid.ID)

	// ********************************************************************************
	log.Entry(ctx).Infof("executing phase 5: create sriovns network service endpoint (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	endpoint := sriovns.NewServer(
		ctx,
		config.Name,
		authorize.NewServer(),
		spiffejwt.TokenGeneratorFunc(source, config.MaxTokenLifetime),
		tokenPool,
		sriovConfig,
		functions,
		binders,
		config.VFIOPath, config.CgroupPath,
		&config.ConnectTo,
		grpc.WithTransportCredentials(
			grpcfd.TransportCredentials(
				credentials.NewTLS(tlsconfig.MTLSClientConfig(source, source, tlsconfig.AuthorizeAny())),
			),
		),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	)

	// ********************************************************************************
	log.Entry(ctx).Infof("executing phase 6: create grpc server and register sriovns (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	// TODO: Add ServerOptions for tracing
	server := grpc.NewServer(grpc.Creds(
		grpcfd.TransportCredentials(
			credentials.NewTLS(tlsconfig.MTLSServerConfig(source, source, tlsconfig.AuthorizeAny())),
		),
	))
	endpoint.Register(server)
	srvErrCh := grpcutils.ListenAndServe(ctx, &config.ListenOn, server)
	exitOnErr(ctx, cancel, srvErrCh)

	// ********************************************************************************
	log.Entry(ctx).Infof("executing phase 7: register %s with the registry (time since start: %s)", config.NSName, time.Since(starttime))
	// ********************************************************************************
	registryCreds := credentials.NewTLS(tlsconfig.MTLSClientConfig(source, source, tlsconfig.AuthorizeAny()))
	registryCreds = grpcfd.TransportCredentials(registryCreds)
	registryCC, err := grpc.DialContext(ctx,
		config.ConnectTo.String(),
		grpc.WithTransportCredentials(registryCreds),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Entry(ctx).Fatalf("failed to connect to registry: %+v", err)
	}

	registryClient := registrychain.NewNetworkServiceEndpointRegistryClient(
		// TODO - add refresh
		registrysendfd.NewNetworkServiceEndpointRegistryClient(),
		registryapi.NewNetworkServiceEndpointRegistryClient(registryCC),
	)
	// TODO - something smarter for expireTime
	expireTime, err := ptypes.TimestampProto(time.Now().Add(config.MaxTokenLifetime))
	if err != nil {
		log.Entry(ctx).Fatalf("failed to connect to registry: %+v", err)
	}
	_, err = registryClient.Register(ctx, &registryapi.NetworkServiceEndpoint{
		Name:                config.Name,
		NetworkServiceNames: []string{config.NSName},
		Url:                 config.ListenOn.String(),
		ExpirationTime:      expireTime,
	})
	if err != nil {
		log.Entry(ctx).Fatalf("failed to connect to registry: %+v", err)
	}

	log.Entry(ctx).Infof("Startup completed in %v", time.Since(starttime))

	<-ctx.Done()
}

func initPCIFunctions(ctx context.Context, sriovConfigFile, pciDevicesPath, pciDriversPath string) (
	sriovConfig *sriovconfig.Config,
	functions map[sriov.PCIFunction][]sriov.PCIFunction,
	binders map[uint][]sriov.DriverBinder,
	err error,
) {
	sriovConfig, err = sriovconfig.ReadConfig(ctx, sriovConfigFile)
	if err != nil {
		log.Entry(ctx).Fatalf("failed to get PCI resources config: %+v", err)
	}

	functions = map[sriov.PCIFunction][]sriov.PCIFunction{}
	binders = map[uint][]sriov.DriverBinder{}
	for pfPCIAddr, pff := range sriovConfig.PhysicalFunctions {
		pff.VirtualFunctions = map[string]uint{}

		pf, err := pcifunction.NewPhysicalFunction(pfPCIAddr, pciDevicesPath, pciDriversPath)
		if err != nil {
			return nil, nil, nil, err
		}
		capacity, err := pf.GetVirtualFunctionsCapacity()
		if err != nil {
			return nil, nil, nil, err
		}
		err = pf.CreateVirtualFunctions(capacity)
		if err != nil {
			return nil, nil, nil, err
		}
		vfs, err := pf.GetVirtualFunctions()
		if err != nil {
			return nil, nil, nil, err
		}

		iommuGroup, err := pf.GetIOMMUGroup()
		if err != nil {
			return nil, nil, nil, err
		}
		binders[iommuGroup] = append(binders[iommuGroup], pf)

		for _, vf := range vfs {
			functions[pf] = append(functions[pf], vf)

			iommuGroup, err := vf.GetIOMMUGroup()
			if err != nil {
				return nil, nil, nil, err
			}
			binders[iommuGroup] = append(binders[iommuGroup], vf)

			pff.VirtualFunctions[vf.GetPCIAddress()] = iommuGroup
		}
	}
	return sriovConfig, functions, binders, nil
}

func exitOnErr(ctx context.Context, cancel context.CancelFunc, errCh <-chan error) {
	// If we already have an error, log it and exit
	select {
	case err := <-errCh:
		log.Entry(ctx).Fatal(err)
	default:
	}
	// Otherwise wait for an error in the background to log and cancel
	go func(ctx context.Context, errCh <-chan error) {
		err := <-errCh
		log.Entry(ctx).Error(err)
		cancel()
	}(ctx, errCh)
}
