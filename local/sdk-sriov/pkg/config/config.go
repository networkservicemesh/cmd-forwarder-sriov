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

// Package config define reading settings from file
package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/types"
)

// ReadConfig read supported ports
func ReadConfig(configFile string) (*types.ResourceConfigList, error) {
	resources := &types.ResourceConfigList{}

	rawBytes, err := ioutil.ReadFile(filepath.Clean(configFile))
	if err != nil {
		return nil, errors.Errorf("error reading file %s, %v", configFile, err)
	}

	if err = json.Unmarshal(rawBytes, resources); err != nil {
		return nil, errors.Errorf("error unmarshalling raw bytes %v", err)
	}

	fmt.Printf("raw ResourceList: %s", rawBytes)
	fmt.Printf("unmarshalled ResourceList: %+v", resources.ResourceList)

	return resources, nil
}
