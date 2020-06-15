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

package resourcepool

import (
	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/types"
	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/utils"
)

type pfDevice struct {
	pciAddr    string
	vfCapacity int
	vfsInUse   []types.VfDevice
	freeVfs    []types.VfDevice
}

func newPfDevice(pciAddr string) types.PfDevice {
	vfCapacity := utils.GetSriovVFcapacity(pciAddr)
	return &pfDevice{
		pciAddr:    pciAddr,
		vfCapacity: vfCapacity,
		vfsInUse:   make([]types.VfDevice, 0),
		freeVfs:    make([]types.VfDevice, 0),
	}
}

func (p pfDevice) GetPciAddr() string {
	return p.pciAddr
}

func (p pfDevice) GetSriovVFCapacity() int {
	return p.vfCapacity
}

func (p pfDevice) GetVFsInUse() []types.VfDevice {
	return p.vfsInUse
}

func (p pfDevice) SetVFsInUse(vfsInUse []types.VfDevice) {
	p.vfsInUse = vfsInUse
}

func (p pfDevice) GetFreeVFs() []types.VfDevice {
	return p.freeVfs
}

func (p pfDevice) SetFreeVFs(freeVfs []types.VfDevice) {
	p.freeVfs = freeVfs
}
