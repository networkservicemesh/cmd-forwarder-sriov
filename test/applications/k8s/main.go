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
	"os"
	"path"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/kelseyhightower/envconfig"
	"github.com/networkservicemesh/sdk/pkg/tools/debug"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/signalctx"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/tools/socketpath"
	"github.com/networkservicemesh/cmd-forwarder-sriov/test/applications/k8s/deviceplugin"
	"github.com/networkservicemesh/cmd-forwarder-sriov/test/applications/k8s/podresources"
)

const kubeletSocket = "kubelet.sock"

// Config - configuration for k8s
type Config struct {
	DevicePluginPath string `default:"/var/lib/kubelet/device-plugins/" desc:"path to the device plugin directory"`
	PodResourcesPath string `default:"/var/lib/kubelet/pod-resources/" desc:"path to the pod resources directory"`
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
	config := &Config{}
	if err := envconfig.Usage("nsm", config); err != nil {
		logrus.Fatal(err)
	}
	if err := envconfig.Process("nsm", config); err != nil {
		logrus.Fatalf("error processing config from env: %+v", err)
	}

	log.Entry(ctx).Infof("Config: %#v", config)

	// Create and start device plugin server
	grpcServer := grpc.NewServer()
	deviceplugin.StartRegistrationServer(config.DevicePluginPath, grpcServer)
	socketPath := socketpath.SocketPath(path.Join(config.DevicePluginPath, kubeletSocket))
	exitOnErr(ctx, cancel, grpcutils.ListenAndServe(ctx, grpcutils.AddressToURL(socketPath), grpcServer))

	// Create and start pod resources server
	grpcServer = grpc.NewServer()
	podresources.StartPodResourcesListerServer(grpcServer)
	socketPath = socketpath.SocketPath(path.Join(config.PodResourcesPath, kubeletSocket))
	exitOnErr(ctx, cancel, grpcutils.ListenAndServe(ctx, grpcutils.AddressToURL(socketPath), grpcServer))

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
