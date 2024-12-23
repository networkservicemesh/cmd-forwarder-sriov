// Copyright (c) 2020 Doc.ai and/or its affiliates.
//
// Copyright (c) 2020-2024 Cisco and/or its affiliates.
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

//go:build linux
// +build linux

package main_test

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

const (
	contextTimeout = 20 * time.Second
)

func (f *ForwarderTestSuite) TestHealthCheck() {
	ctx, cancel := context.WithTimeout(f.ctx, contextTimeout)
	defer cancel()

	healthClient := grpc_health_v1.NewHealthClient(f.sutCC)
	healthResponse, err := healthClient.Check(ctx,
		&grpc_health_v1.HealthCheckRequest{
			Service: "networkservice.NetworkService",
		},
		grpc.WaitForReady(true),
	)
	f.NoError(err)
	f.Require().NotNil(healthResponse)
	f.Equal(grpc_health_v1.HealthCheckResponse_SERVING, healthResponse.Status)
}
