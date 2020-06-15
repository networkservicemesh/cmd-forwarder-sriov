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

package utils

import (
	"bytes"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

var (
	sysBusPci = "/sys/bus/pci/devices"
)

const (
	totalVfFile      = "sriov_totalvfs"
	configuredVfFile = "sriov_numvfs"
)

// IsSriovCapable check if a pci device SR-IOV capable given its pci address
func IsSriovCapable(pciAddr string) bool {
	totalVfFilePath := filepath.Join(sysBusPci, pciAddr, totalVfFile)
	if _, err := os.Stat(totalVfFilePath); err != nil {
		return false
	}
	// sriov_totalvfs file exists -> sriov capable
	return true
}

// IsSriovVF check if a pci device has link to a PF
func IsSriovVF(pciAddr string) bool {
	totalVfFilePath := filepath.Join(sysBusPci, pciAddr, "physfn")
	if _, err := os.Stat(totalVfFilePath); err != nil {
		return false
	}
	return true
}

// GetVFconfigured returns number of VF configured for a PF
func GetVFconfigured(pf string) int {
	configuredVfPath := filepath.Join(sysBusPci, pf, configuredVfFile)
	vfs, err := ioutil.ReadFile(configuredVfPath)
	if err != nil {
		return 0
	}
	configuredVFs := bytes.TrimSpace(vfs)
	numConfiguredVFs, err := strconv.Atoi(string(configuredVFs))
	if err != nil {
		return 0
	}
	return numConfiguredVFs
}

// IsSriovConfigured returns true if sriov_numvfs reads > 0 else false
func IsSriovConfigured(addr string) bool {
	if GetVFconfigured(addr) > 0 {
		return true
	}
	return false
}

// GetSriovVFcapacity returns SRIOV VF capacity - number of VFs that can be created for specified PF
func GetSriovVFcapacity(pfPciAddr string) int {
	totalVfFilePath := filepath.Join(sysBusPci, pfPciAddr, totalVfFile)
	vfs, err := ioutil.ReadFile(totalVfFilePath)
	if err != nil {
		return 0
	}
	totalvfs := bytes.TrimSpace(vfs)
	numvfs, err := strconv.Atoi(string(totalvfs))
	if err != nil {
		return 0
	}
	return numvfs
}

// DeviceExists validates PciAddr given as string and checks if it exists
func DeviceExists(pciAddr string) error {
	//Check system pci address

	// sysbus pci address regex
	var validLongID = regexp.MustCompile(`^0{4}:[0-9a-f]{2}:[0-9a-f]{2}.[0-7]{1}$`)
	var validShortID = regexp.MustCompile(`^[0-9a-f]{2}:[0-9a-f]{2}.[0-7]{1}$`)

	if validLongID.MatchString(pciAddr) {
		return deviceExist(pciAddr)
	} else if validShortID.MatchString(pciAddr) {
		pciAddr = "0000:" + pciAddr // make short form to sysfs represtation
		return deviceExist(pciAddr)
	}
	return fmt.Errorf("invalid pci address %s", pciAddr)
}

func deviceExist(addr string) error {
	devPath := filepath.Join(sysBusPci, addr)
	_, err := os.Lstat(devPath)
	if err != nil {
		return fmt.Errorf("unable to read device directory %s", devPath)
	}
	return nil
}

// GetNetNames returns host net interface names as string for a PCI device from its pci address
func GetNetNames(pciAddr string) ([]string, error) {
	var names []string
	netDir := filepath.Join(sysBusPci, pciAddr, "net")
	if _, err := os.Lstat(netDir); err != nil {
		return nil, fmt.Errorf("GetNetName(): no net directory under pci device %s: %q", pciAddr, err)
	}

	fInfos, err := ioutil.ReadDir(netDir)
	if err != nil {
		return nil, fmt.Errorf("GetNetName(): failed to read net directory %s: %q", netDir, err)
	}

	names = make([]string, 0)
	for _, f := range fInfos {
		names = append(names, f.Name())
	}
	return names, nil
}

// IsDefaultRoute returns true if PCI network device is default route interface
func IsDefaultRoute(pciAddr string) (bool, error) {
	// Get net interface name
	ifNames, err := GetNetNames(pciAddr)
	if err != nil {
		return false, fmt.Errorf("error trying get net device name for device %s", pciAddr)
	}

	if len(ifNames) > 0 { // there's at least one interface name found
		for _, ifName := range ifNames {
			link, err := netlink.LinkByName(ifName)
			if err != nil {
				logrus.Errorf("expected to get valid host interface with name %s: %q", ifName, err)
			}

			routes, err := netlink.RouteList(link, netlink.FAMILY_V4) // IPv6 routes: all interface has at least one link local route entry
			for _, r := range routes {
				if r.Dst == nil {
					logrus.Infof("excluding interface %s:  default route found: %+v", ifName, r)
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// CreateVFs initializes VFs for specified PF given number of VFs
func CreateVFs(pfPciAddr string, vfNumber int) error {
	configuredVfPath := filepath.Join(sysBusPci, pfPciAddr, configuredVfFile)
	err := ioutil.WriteFile(configuredVfPath, []byte(strconv.FormatInt(int64(vfNumber), 10)), 0x666)
	if err != nil {
		return err
	}
	return nil
}

// GetVFList returns a List containing PCI addr for all VF discovered in a given PF
func GetVFList(pfPciAddr string) (vfList []string, err error) {
	vfList = make([]string, 0)
	pfDir := filepath.Join(sysBusPci, pfPciAddr)
	_, err = os.Lstat(pfDir)
	if err != nil {
		err = fmt.Errorf("could not get PF directory information for device %s: %v", pfPciAddr, err)
		return
	}

	vfDirs, err := filepath.Glob(filepath.Join(pfDir, "virtfn*"))
	if err != nil {
		err = fmt.Errorf("error reading VF directories %v", err)
		return
	}

	//Read all VF directory and get add VF PCI addr to the vfList
	for _, dir := range vfDirs {
		dirInfo, err := os.Lstat(dir)
		if err == nil && (dirInfo.Mode()&os.ModeSymlink != 0) {
			linkName, err := filepath.EvalSymlinks(dir)
			if err == nil {
				vfLink := filepath.Base(linkName)
				vfList = append(vfList, vfLink)
			}
		}
	}
	return
}
