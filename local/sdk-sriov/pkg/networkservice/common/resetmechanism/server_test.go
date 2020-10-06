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

package resetmechanism_test

import (
	"context"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/chain"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/next"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/networkservice/common/resetmechanism"
)

const (
	mech1 = "mech-1"
	mech2 = "mech-2"
)

func TestResetMechanismServer_Request(t *testing.T) {
	mechs := map[string]bool{}

	m := &mock.Mock{}
	m.On("Request", mock.Anything, mock.Anything).
		Return(nil, nil)
	m.On("Close", mock.Anything, mock.Anything).
		Return(nil, nil)

	server := chain.NewNetworkServiceServer(
		resetmechanism.NewServer(&mechChainElement{mechs}),
		&mockChainElement{m},
	)

	// 1. Request with mech1 mechanism

	request := &networkservice.NetworkServiceRequest{
		Connection: &networkservice.Connection{
			Id: "test-ID",
		},
		MechanismPreferences: []*networkservice.Mechanism{
			{
				Type: mech1,
			},
		},
	}

	conn, err := server.Request(context.TODO(), request)
	require.NoError(t, err)
	require.Equal(t, mech1, conn.Mechanism.Type)

	require.True(t, mechs[mech1])

	m.AssertNumberOfCalls(t, "Request", 1)
	m.AssertNumberOfCalls(t, "Close", 0)

	// 2. Request with mech2 mechanism

	request.Connection.Mechanism = nil
	request.MechanismPreferences[0].Type = mech2

	conn, err = server.Request(context.TODO(), request)
	require.NoError(t, err)
	require.Equal(t, mech2, conn.Mechanism.Type)

	require.False(t, mechs[mech1])
	require.True(t, mechs[mech2])

	m.AssertNumberOfCalls(t, "Request", 2)
	m.AssertNumberOfCalls(t, "Close", 0)

	// 3. Close

	_, err = server.Close(context.TODO(), conn)
	require.NoError(t, err)

	require.False(t, mechs[mech1])
	require.False(t, mechs[mech2])

	m.AssertNumberOfCalls(t, "Request", 2)
	m.AssertNumberOfCalls(t, "Close", 1)
}

type mechChainElement struct {
	mechs map[string]bool
}

func (m *mechChainElement) Request(ctx context.Context, request *networkservice.NetworkServiceRequest) (*networkservice.Connection, error) {
	mech := request.GetMechanismPreferences()[0]
	request.GetConnection().Mechanism = mech
	m.mechs[mech.GetType()] = true

	return next.Server(ctx).Request(ctx, request)
}

func (m *mechChainElement) Close(ctx context.Context, conn *networkservice.Connection) (*empty.Empty, error) {
	mech := conn.GetMechanism()
	m.mechs[mech.GetType()] = false

	return next.Server(ctx).Close(ctx, conn)
}

type mockChainElement struct {
	mock *mock.Mock
}

func (f *mockChainElement) Request(ctx context.Context, request *networkservice.NetworkServiceRequest) (*networkservice.Connection, error) {
	f.mock.Called(ctx, request)
	return next.Server(ctx).Request(ctx, request)
}

func (f *mockChainElement) Close(ctx context.Context, conn *networkservice.Connection) (*empty.Empty, error) {
	f.mock.Called(ctx, conn)
	return next.Server(ctx).Close(ctx, conn)
}
