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

// Package resetmechanism provides wrapper chain element to reset underlying server on mechanism change
package resetmechanism

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/chain"
)

type resetMechanismServer struct {
	wrappedServer  networkservice.NetworkServiceServer
	connMechanisms map[string]*networkservice.Mechanism
}

// NewServer returns a new reset mechanism server chain element
func NewServer(wrappedServer networkservice.NetworkServiceServer) networkservice.NetworkServiceServer {
	return &resetMechanismServer{
		wrappedServer:  wrappedServer,
		connMechanisms: map[string]*networkservice.Mechanism{},
	}
}

func (s *resetMechanismServer) Request(ctx context.Context, request *networkservice.NetworkServiceRequest) (*networkservice.Connection, error) {
	connID := request.GetConnection().GetId()

	if storedMech, ok := s.connMechanisms[connID]; ok {
		if mech := request.GetConnection().GetMechanism(); mech == nil || mech.GetType() != storedMech.GetType() {
			conn := request.GetConnection().Clone()
			conn.Mechanism = storedMech
			// we need to close only the wrapped part, not whole chain
			if _, err := chain.NewNetworkServiceServer(
				s.wrappedServer,
				&dummyServer{},
			).Close(ctx, conn); err != nil {
				return nil, err
			}
		}
	}

	conn, err := s.wrappedServer.Request(ctx, request)
	if err != nil {
		return nil, err
	}

	if mech := conn.GetMechanism(); mech != nil {
		s.connMechanisms[connID] = mech.Clone()
	}

	return conn, nil
}

func (s *resetMechanismServer) Close(ctx context.Context, conn *networkservice.Connection) (*empty.Empty, error) {
	delete(s.connMechanisms, conn.GetId())

	return s.wrappedServer.Close(ctx, conn)
}
