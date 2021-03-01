// Copyright (c) 2020-2021 Doc.ai and/or its affiliates.
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

package main

import (
	"context"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/edwarnicke/grpcfd"
	"github.com/kelseyhightower/envconfig"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	registryapi "github.com/networkservicemesh/api/pkg/api/registry"
	k8sdeviceplugin "github.com/networkservicemesh/sdk-k8s/pkg/tools/deviceplugin"
	k8spodresources "github.com/networkservicemesh/sdk-k8s/pkg/tools/podresources"
	"github.com/networkservicemesh/sdk-sriov/pkg/networkservice/chains/xconnectns"
	sriovconfig "github.com/networkservicemesh/sdk-sriov/pkg/sriov/config"
	"github.com/networkservicemesh/sdk-sriov/pkg/sriov/pci"
	"github.com/networkservicemesh/sdk-sriov/pkg/sriov/resource"
	sriovtoken "github.com/networkservicemesh/sdk-sriov/pkg/sriov/token"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/authorize"
	registryclient "github.com/networkservicemesh/sdk/pkg/registry/chains/client"
	"github.com/networkservicemesh/sdk/pkg/tools/debug"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	"github.com/networkservicemesh/sdk/pkg/tools/jaeger"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/log/logruslogger"
	"github.com/networkservicemesh/sdk/pkg/tools/opentracing"
	"github.com/networkservicemesh/sdk/pkg/tools/signalctx"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"
	"github.com/networkservicemesh/sdk/pkg/tools/token"

	"github.com/networkservicemesh/cmd-forwarder-sriov/internal/deviceplugin"
)

// Config - configuration for cmd-forwarder-sriov
type Config struct {
	Name                string        `default:"sriov-forwarder" desc:"name of Endpoint"`
	NSName              string        `default:"sriovns" desc:"Name of Network Service to Register with Registry"`
	ConnectTo           url.URL       `default:"unix:///var/lib/networkservicemesh/nsm.io.sock" desc:"URL to connect to" split_words:"true"`
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
	ctx = log.WithFields(ctx, map[string]interface{}{"cmd": os.Args[0]})
	ctx = log.WithLog(ctx, logruslogger.New(ctx))

	// ********************************************************************************
	// Configure open tracing
	// ********************************************************************************
	log.EnableTracing(true)
	jaegerCloser := jaeger.InitJaeger(ctx, "cmd-forwarder-sriov")
	defer func() { _ = jaegerCloser.Close() }()

	// ********************************************************************************
	// Debug self if necessary
	// ********************************************************************************
	if err := debug.Self(); err != nil {
		log.FromContext(ctx).Infof("%s", err)
	}

	starttime := time.Now()

	// enumerating phases
	log.FromContext(ctx).Infof("there are 8 phases which will be executed followed by a success message:")
	log.FromContext(ctx).Infof("the phases include:")
	log.FromContext(ctx).Infof("1: get config from environment")
	log.FromContext(ctx).Infof("2: get SR-IOV config from file")
	log.FromContext(ctx).Infof("3: init pools")
	log.FromContext(ctx).Infof("4: start device plugin server")
	log.FromContext(ctx).Infof("5: retrieve spiffe svid")
	log.FromContext(ctx).Infof("6: create sriovns network service endpoint")
	log.FromContext(ctx).Infof("7: create grpc server and register sriovns")
	log.FromContext(ctx).Infof("8: register xconnectns with the registry")
	log.FromContext(ctx).Infof("a final success message with start time duration")

	// ********************************************************************************
	log.FromContext(ctx).Infof("executing phase 1: get config from environment (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	config := &Config{}
	if err := envconfig.Usage("nsm", config); err != nil {
		log.FromContext(ctx).Fatal(err)
	}
	if err := envconfig.Process("nsm", config); err != nil {
		log.FromContext(ctx).Fatalf("error processing config from env: %+v", err)
	}

	log.FromContext(ctx).Infof("Config: %#v", config)

	// ********************************************************************************
	log.FromContext(ctx).Infof("executing phase 2: get SR-IOV config from file (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	sriovConfig, err := sriovconfig.ReadConfig(ctx, config.SRIOVConfigFile)
	if err != nil {
		log.FromContext(ctx).Fatalf("failed to get PCI resources config: %+v", err)
	}

	if err = pci.UpdateConfig(config.PCIDevicesPath, config.PCIDriversPath, sriovConfig); err != nil {
		log.FromContext(ctx).Fatalf("failed to update PCI resources config with VFs: %+v", err)
	}

	// ********************************************************************************
	log.FromContext(ctx).Infof("executing phase 3: init pools (time since start: %s)", time.Since(starttime))
	// ********************************************************************************

	tokenPool := sriovtoken.NewPool(sriovConfig)

	pciPool, err := pci.NewPool(config.PCIDevicesPath, config.PCIDriversPath, config.VFIOPath, sriovConfig)
	if err != nil {
		log.FromContext(ctx).Fatalf("failed to init PCI pool: %+v", err)
	}

	resourcePool := resource.NewPool(tokenPool, sriovConfig)

	// ********************************************************************************
	log.FromContext(ctx).Infof("executing phase 4: start device plugin server (time since start: %s)", time.Since(starttime))
	// ********************************************************************************

	// Start device plugin server
	if err = deviceplugin.StartServers(
		ctx,
		tokenPool,
		config.ResourcePollTimeout,
		k8sdeviceplugin.NewClient(config.DevicePluginPath),
		k8spodresources.NewClient(config.PodResourcesPath),
	); err != nil {
		log.FromContext(ctx).Fatalf("failed to start a device plugin server: %+v", err)
	}

	// ********************************************************************************
	log.FromContext(ctx).Infof("executing phase 5: retrieving svid, check spire agent logs if this is the last line you see (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	source, err := workloadapi.NewX509Source(ctx)
	if err != nil {
		log.FromContext(ctx).Fatalf("error getting x509 source: %+v", err)
	}
	svid, err := source.GetX509SVID()
	if err != nil {
		log.FromContext(ctx).Fatalf("error getting x509 svid: %+v", err)
	}
	log.FromContext(ctx).Infof("SVID: %q", svid.ID)

	// ********************************************************************************
	log.FromContext(ctx).Infof("executing phase 6: create sriovns network service endpoint (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	endpoint := xconnectns.NewServer(
		ctx,
		config.Name,
		authorize.NewServer(),
		spiffejwt.TokenGeneratorFunc(source, config.MaxTokenLifetime),
		pciPool,
		resourcePool,
		sriovConfig,
		config.VFIOPath, config.CgroupPath,
		&config.ConnectTo,
		grpc.WithTransportCredentials(
			grpcfd.TransportCredentials(
				credentials.NewTLS(tlsconfig.MTLSClientConfig(source, source, tlsconfig.AuthorizeAny())),
			),
		),
		grpc.WithDefaultCallOptions(
			grpc.WaitForReady(true),
			grpc.PerRPCCredentials(token.NewPerRPCCredentials(spiffejwt.TokenGeneratorFunc(source, config.MaxTokenLifetime))),
		),
		grpcfd.WithChainStreamInterceptor(),
		grpcfd.WithChainUnaryInterceptor(),
	)

	// ********************************************************************************
	log.FromContext(ctx).Infof("executing phase 7: create grpc server and register sriovns (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	tmpDir, err := ioutil.TempDir("", "sriov-forwarder")
	if err != nil {
		log.FromContext(ctx).Fatalf("error creating tmpDir: %+v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	listenOn := &url.URL{Scheme: "unix", Path: path.Join(tmpDir, "listen_on.io.sock")}

	server := grpc.NewServer(append(
		opentracing.WithTracing(),
		grpc.Creds(
			grpcfd.TransportCredentials(
				credentials.NewTLS(tlsconfig.MTLSServerConfig(source, source, tlsconfig.AuthorizeAny())),
			),
		))...)
	endpoint.Register(server)
	srvErrCh := grpcutils.ListenAndServe(ctx, listenOn, server)
	exitOnErr(ctx, cancel, srvErrCh)

	// ********************************************************************************
	log.FromContext(ctx).Infof("executing phase 8: register %s with the registry (time since start: %s)", config.NSName, time.Since(starttime))
	// ********************************************************************************
	clientOptions := append(
		opentracing.WithTracingDial(),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
		grpc.WithTransportCredentials(
			grpcfd.TransportCredentials(
				credentials.NewTLS(
					tlsconfig.MTLSClientConfig(source, source, tlsconfig.AuthorizeAny()),
				),
			),
		),
	)
	registryCC, err := grpc.DialContext(ctx,
		grpcutils.URLToTarget(&config.ConnectTo),
		clientOptions...,
	)
	if err != nil {
		log.FromContext(ctx).Fatalf("failed to connect to registry: %+v", err)
	}

	registryClient := registryclient.NewNetworkServiceEndpointRegistryInterposeClient(ctx, registryCC)
	_, err = registryClient.Register(ctx, &registryapi.NetworkServiceEndpoint{
		Name:                config.Name,
		NetworkServiceNames: []string{config.NSName},
		Url:                 grpcutils.URLToTarget(listenOn),
	})
	if err != nil {
		log.FromContext(ctx).Fatalf("failed to connect to registry: %+v", err)
	}

	log.FromContext(ctx).Infof("Startup completed in %v", time.Since(starttime))

	<-ctx.Done()
}

func exitOnErr(ctx context.Context, cancel context.CancelFunc, errCh <-chan error) {
	// If we already have an error, log it and exit
	select {
	case err := <-errCh:
		log.FromContext(ctx).Fatal(err)
	default:
	}
	// Otherwise wait for an error in the background to log and cancel
	go func(ctx context.Context, errCh <-chan error) {
		err := <-errCh
		log.FromContext(ctx).Error(err)
		cancel()
	}(ctx, errCh)
}
