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
	"flag"
	"fmt"
	"net/url"
	"os"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/kelseyhightower/envconfig"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"google.golang.org/grpc/credentials"

	"github.com/networkservicemesh/sdk/pkg/networkservice/common/authorize"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"github.com/networkservicemesh/sdk/pkg/tools/debug"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/signalctx"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/config"
	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/networkservice/chains/sriov"
	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/resourcepool"
)

// Config - configuration for cmd-forwarder-sriov
type Config struct {
	Name             string        `default:"forwarder" desc:"Name of Endpoint"`
	ListenOn         url.URL       `default:"unix:///listen.on.socket" desc:"url to listen on" split_words:"true"`
	ConnectTo        url.URL       `default:"unix:///connect.to.socket" desc:"url to connect to" split_words:"true"`
	MaxTokenLifetime time.Duration `default:"24h" desc:"maximum lifetime of tokens" split_words:"true"`
}

const (
	defaultConfig = "/etc/nsm/sriov/config.yaml"
)

type cliFlags struct {
	configFile string
}

func main() {
	// Setup context to catch signals
	ctx := signalctx.WithSignals(context.Background())
	ctx, cancel := context.WithCancel(ctx)

	// Setup logging
	logrus.SetFormatter(&nested.Formatter{})
	logrus.SetLevel(logrus.TraceLevel)
	ctx = log.WithField(ctx, "cmd", os.Args[0])

	// Debug self if necessary
	if err := debug.Self(); err != nil {
		log.Entry(ctx).Infof("%s", err)
	}

	starttime := time.Now()

	// Get config from environment
	cfg := &Config{}
	if err := envconfig.Usage("nsm", cfg); err != nil {
		logrus.Fatal(err)
	}
	if err := envconfig.Process("nsm", cfg); err != nil {
		logrus.Fatalf("error processing config from env: %+v", err)
	}

	log.Entry(ctx).Infof("Config: %#v", cfg)

	// Get a X509Source
	source, err := workloadapi.NewX509Source(ctx)
	if err != nil {
		logrus.Fatalf("error getting x509 source: %+v", err)
	}
	svid, err := source.GetX509SVID()
	if err != nil {
		logrus.Fatalf("error getting x509 svid: %+v", err)
	}
	logrus.Infof("SVID: %q", svid.ID)

	resourceCfg, err := getResourceConfig()
	if err != nil {
		logrus.Fatalf("Error getting resource config: %v", err)
	}

	resourcePool := resourcepool.NewNetResourcePool()
	err = resourcePool.AddNetDevices(resourceCfg)
	if err != nil {
		logrus.Fatalf("error processing devices from config: %v", err)
	}

	// XConnect Network Service Endpoint
	endpoint := sriov.NewServer(
		cfg.Name,
		resourcePool,
		authorize.NewServer(),
		spiffejwt.TokenGeneratorFunc(source, cfg.MaxTokenLifetime),
		&cfg.ConnectTo,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsconfig.MTLSClientConfig(source, source, tlsconfig.AuthorizeAny()))),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	)

	// Create GRPC Server
	// TODO - add ServerOptions for Tracing
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsconfig.MTLSServerConfig(source, source, tlsconfig.AuthorizeAny()))))
	endpoint.Register(server)
	srvErrCh := grpcutils.ListenAndServe(ctx, &cfg.ListenOn, server)
	exitOnErr(ctx, cancel, srvErrCh)
	log.Entry(ctx).Infof("Startup completed in %v", time.Since(starttime))

	<-ctx.Done()
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

func getResourceConfig() (*config.ResourceConfigList, error) {
	cf := &cliFlags{}
	flag.StringVar(&cf.configFile, "config-file", defaultConfig, "YAML device pool config file location")
	flag.Parse()

	logrus.Infof("reading configs")
	resourceCfg, err := config.ReadConfig(cf.configFile)

	if err != nil {
		return nil, fmt.Errorf("error getting config from file %v", cf.configFile)
	}

	if len(resourceCfg.ResourceList) < 1 {
		return nil, fmt.Errorf("no resource configuration provided")
	}

	return resourceCfg, nil
}
