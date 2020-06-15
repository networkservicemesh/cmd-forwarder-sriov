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

// Package config contains types and methods for parsing forwarder's net device pool config
package config

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"
)

// ResourceConfigList is list of ResourceConfig
type ResourceConfigList struct {
	ResourceList []ResourceConfig `yaml:"resourceList"` // config file: "resourceList" :[{<ResourceConfig configs>},{},{},...]
}

// ResourceConfig contains configuration parameters for a resource pool
type ResourceConfig struct {
	RegistryDomainName string `yaml:"registryDomainName"`
	Capability         string `yaml:"capability"`
	DevicePciAddress   string `yaml:"devicePciAddress"`
	ConnectedToPort    string `yaml:"connectedToPort"`
}

// ReadConfig reads and parses config by provided configuration file path
func ReadConfig(configFile string) (*ResourceConfigList, error) {
	resources := &ResourceConfigList{}

	rawBytes, err := ioutil.ReadFile(filepath.Clean(configFile))
	if err != nil {
		return nil, fmt.Errorf("error reading file %s, %v", configFile, err)
	}

	if err = yaml.Unmarshal(rawBytes, resources); err != nil {
		return nil, fmt.Errorf("error unmarshalling raw bytes %v", err)
	}

	logrus.Infof("raw ResourceList: %s", rawBytes)
	logrus.Infof("unmarshalled ResourceList: %+v", resources.ResourceList)

	return resources, nil
}
