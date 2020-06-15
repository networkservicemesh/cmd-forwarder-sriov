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

package resource_pool

import (
	"fmt"
	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/config"
	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/types"
	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/utils"
)

type netResourcePool struct {
	entries []types.NetResource
}

// NewNetResourcePool is NetResourcePool implementation from netResourcePool instance
func NewNetResourcePool() types.NetResourcePool {
	return &netResourcePool{
		entries: make([]types.NetResource, 0),
	}
}

func (rp *netResourcePool) AddNetDevices(resourceConfig *config.ResourceConfigList) error {
	for _, cfg := range resourceConfig.ResourceList {
		pciAddr := cfg.DevicePciAddress

		err := rp.validateDevice(pciAddr)
		if err != nil {
			return fmt.Errorf("invalid device: %v", err)
		}

		// TODO also check capability by checking device.GetLinkSpeed???
		pfDevice := newPfDevice(pciAddr)
		err = utils.CreateVFs(pciAddr, pfDevice.GetSriovVFCapacity())
		if err != nil {
			return fmt.Errorf("unable to create vitual functions for device %s: %v", pciAddr, err)
		}

		vfs, err := utils.GetVFList(pciAddr)
		if err != nil {
			return fmt.Errorf("unable to discover vitual functions for device %s: %v", pciAddr, err)
		}
		freeVfs := make([]types.VfDevice, 0)
		for _, vf := range vfs {
			freeVfs = append(freeVfs, newVfDevice(vf))
		}
		pfDevice.SetFreeVFs(freeVfs)
	}
	return nil
}

func (rp *netResourcePool) GetEntries() []types.NetResource {
	return rp.entries
}

func (rp *netResourcePool) validateDevice(pciAddr string) error {
	if err := utils.DeviceExists(pciAddr); err != nil {
		return err
	}

	if !utils.IsSriovCapable(pciAddr) {
		return fmt.Errorf("device %s is not SR-IOV capable", pciAddr)
	}

	// TODO think about what we do with already configured devices
	if utils.IsSriovConfigured(pciAddr) {
		return fmt.Errorf("device %s is alredy configured", pciAddr)
	}

	// exclude net device in-use in host
	if isDefaultRoute, _ := utils.IsDefaultRoute(pciAddr); isDefaultRoute {
		return fmt.Errorf("device %s is in-use in host", pciAddr)
	}

	return nil
}
