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

package main_test

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/edwarnicke/exechelper"
	"github.com/kelseyhightower/envconfig"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	"github.com/networkservicemesh/sdk/pkg/tools/spire"

	main "github.com/networkservicemesh/cmd-forwarder-sriov"
	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/k8s/k8stest/deviceplugin"
	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/k8s/k8stest/podresources"
	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/tools/socketpath"
)

const (
	kubeletSocket = "kubelet.sock"
)

type ForwarderTestSuite struct {
	suite.Suite
	ctx        context.Context
	cancel     context.CancelFunc
	x509source x509svid.Source
	x509bundle x509bundle.Source
	config     main.Config
	spireErrCh <-chan error
	sutErrCh   <-chan error
}

func (f *ForwarderTestSuite) SetupSuite() {
	logrus.SetFormatter(&nested.Formatter{})
	logrus.SetLevel(logrus.TraceLevel)
	f.ctx, f.cancel = context.WithCancel(context.Background())

	// Run spire
	executable, err := os.Executable()
	require.NoError(f.T(), err)
	f.spireErrCh = spire.Start(
		spire.WithContext(f.ctx),
		spire.WithEntry("spiffe://example.org/forwarder", "unix:path:/bin/forwarder"),
		spire.WithEntry(fmt.Sprintf("spiffe://example.org/%s", filepath.Base(executable)),
			fmt.Sprintf("unix:path:%s", executable),
		),
	)
	require.Len(f.T(), f.spireErrCh, 0)

	// Get X509Source
	source, err := workloadapi.NewX509Source(f.ctx)
	f.x509source = source
	f.x509bundle = source
	require.NoError(f.T(), err)
	svid, err := f.x509source.GetX509SVID()
	if err != nil {
		logrus.Fatalf("error getting x509 svid: %+v", err)
	}
	logrus.Infof("SVID: %q", svid.ID)

	// Get config from env
	require.NoError(f.T(), envconfig.Process("nsm", &f.config))

	// Setup k8s stubs
	f.setupK8sStubs()

	// Run system under test (sut)
	cmdStr := "forwarder"
	f.sutErrCh = exechelper.Start(cmdStr,
		exechelper.WithContext(f.ctx),
		exechelper.WithEnvirons(os.Environ()...),
		exechelper.WithStdout(os.Stdout),
		exechelper.WithStderr(os.Stderr),
	)
	require.Len(f.T(), f.sutErrCh, 0)
}

func (f *ForwarderTestSuite) setupK8sStubs() {
	// Create and start device plugin server
	grpcServer := grpc.NewServer()
	deviceplugin.StartRegistrationServer(f.config.DevicePluginPath, grpcServer)
	socketPath := socketpath.SocketPath(path.Join(f.config.DevicePluginPath, kubeletSocket))
	require.Len(f.T(), grpcutils.ListenAndServe(f.ctx, grpcutils.AddressToURL(socketPath), grpcServer), 1)

	// Create and start pod resources server
	grpcServer = grpc.NewServer()
	podresources.StartPodResourcesListerServer(grpcServer)
	socketPath = socketpath.SocketPath(path.Join(f.config.PodResourcesPath, kubeletSocket))
	require.Len(f.T(), grpcutils.ListenAndServe(f.ctx, grpcutils.AddressToURL(socketPath), grpcServer), 1)
}

func (f *ForwarderTestSuite) TearDownSuite() {
	f.cancel()
	for {
		_, ok := <-f.sutErrCh
		if !ok {
			break
		}
	}
	for {
		_, ok := <-f.spireErrCh
		if !ok {
			break
		}
	}
}

func (f *ForwarderTestSuite) TestHealthCheck() {
	ctx, cancel := context.WithTimeout(f.ctx, 100*time.Second)
	defer cancel()
	// TODO - this is where we fail.  Check to see if spire-agent is wired up correctly.
	healthCC, err := grpc.DialContext(ctx,
		f.config.ListenOn.String(),
		grpc.WithTransportCredentials(credentials.NewTLS(tlsconfig.MTLSClientConfig(f.x509source, f.x509bundle, tlsconfig.AuthorizeAny()))),
	)
	if err != nil {
		logrus.Fatalf("Failed healthcheck: %+v", err)
	}
	healthClient := grpc_health_v1.NewHealthClient(healthCC)
	healthResponse, err := healthClient.Check(ctx,
		&grpc_health_v1.HealthCheckRequest{
			Service: "networkservice.NetworkService",
		},
		grpc.WaitForReady(true),
	)
	f.NoError(err)
	require.NotNil(f.T(), healthResponse)
	f.Equal(grpc_health_v1.HealthCheckResponse_SERVING, healthResponse.Status)
}

func TestForwarderTestSuite(t *testing.T) {
	suite.Run(t, new(ForwarderTestSuite))
}
