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

package types

import "github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/config"

type NetResourcePool interface {
	AddNetDevices(cfg *config.ResourceConfigList) error
	GetEntries() []NetResource
}

// NetResource contains information about concrete resource in net resource pool
type NetResource struct {
	RegistryDomainName string
	Capability         string
	PfDevice           PfDevice
	ConnectedToPort    string
}

// PfDevice provides an interface to get information about physical function
type PfDevice interface {
	GetPciAddr() string
	GetSriovVFCapacity() int
	GetVFsInUse() []VfDevice
	SetVFsInUse([]VfDevice)
	GetFreeVFs() []VfDevice
	SetFreeVFs([]VfDevice)
}

type VfDevice interface {
	GetPciAddr() string
}
