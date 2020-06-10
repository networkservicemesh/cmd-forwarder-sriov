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

// Package imports used for priming Docker builds to maximize layer caching
package imports

import (
	_ "context" // we need it
	_ "fmt"
	_ "io"
	_ "io/ioutil"
	_ "net"
	_ "net/url"
	_ "os"
	_ "path/filepath"
	_ "testing"
	_ "time"

	_ "github.com/antonfisher/nested-logrus-formatter" // we need it
	_ "github.com/edwarnicke/exechelper"
	_ "github.com/kelseyhightower/envconfig"
	_ "github.com/networkservicemesh/sdk/pkg/tools/debug"
	_ "github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	_ "github.com/networkservicemesh/sdk/pkg/tools/log"
	_ "github.com/networkservicemesh/sdk/pkg/tools/signalctx"
	_ "github.com/networkservicemesh/sdk/pkg/tools/spire"
	_ "github.com/pkg/errors"
	_ "github.com/sirupsen/logrus"
	_ "github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	_ "github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	_ "github.com/spiffe/go-spiffe/v2/svid/x509svid"
	_ "github.com/spiffe/go-spiffe/v2/workloadapi"
	_ "github.com/stretchr/testify/require"
	_ "github.com/stretchr/testify/suite"
	_ "google.golang.org/grpc"
	_ "google.golang.org/grpc/health/grpc_health_v1"
)
