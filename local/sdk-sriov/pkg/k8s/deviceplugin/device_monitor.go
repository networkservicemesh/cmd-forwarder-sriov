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

package deviceplugin

import (
	"context"
	"reflect"
	"sort"
	"time"

	"github.com/networkservicemesh/sdk/pkg/tools/log"
	podresources "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/k8s"
)

const pollingTimeout = 1 * time.Second

// DeviceMonitor is an interface to monitor device changes for the given resource name
type DeviceMonitor interface {
	Monitor(resourceName string) (devicesCh chan []string, errCh chan error)
}

type deviceMonitor struct {
	resourceListerClient podresources.PodResourcesListerClient
	ctx                  context.Context
	devices              map[string][]string
}

func newDeviceMonitor(ctx context.Context, manager k8s.Manager) (DeviceMonitor, error) {
	resourceListerClient, err := manager.GetPodResourcesListerClient(ctx)
	if err != nil {
		return nil, err
	}

	return &deviceMonitor{
		resourceListerClient: resourceListerClient,
		ctx:                  ctx,
		devices:              map[string][]string{},
	}, nil
}

func (dm *deviceMonitor) Monitor(resourceName string) (devicesCh chan []string, errCh chan error) {
	logEntry := log.Entry(dm.ctx).WithField("deviceMonitor", "Monitor")

	devicesCh = make(chan []string)
	errCh = make(chan error)
	go func() {
		logEntry.Infof("start monitoring %v devices", resourceName)
		defer logEntry.Infof("stop monitoring %v devices", resourceName)
		defer close(devicesCh)
		defer close(errCh)

		for {
			resp, err := dm.resourceListerClient.List(dm.ctx, &podresources.ListPodResourcesRequest{})
			if err != nil {
				logEntry.Warnf("resourceListerClient unavailable: %+v", err)
				errCh <- err
				return
			}

			if dm.updateResources(resourceName, resp.PodResources) {
				select {
				case <-dm.ctx.Done():
					return
				case devicesCh <- dm.devices[resourceName]:
				}
			} else {
				select {
				case <-dm.ctx.Done():
					return
				case <-time.After(pollingTimeout):
				}
			}
		}
	}()

	return devicesCh, errCh
}

func (dm *deviceMonitor) updateResources(resourceName string, podResources []*podresources.PodResources) bool {
	var newDevices []string
	for _, pod := range podResources {
		for _, container := range pod.Containers {
			for _, device := range container.Devices {
				if device.ResourceName == resourceName {
					newDevices = append(newDevices, device.DeviceIds...)
				}
			}
		}
	}

	sort.Strings(newDevices)
	shouldUpdate := !reflect.DeepEqual(newDevices, dm.devices[resourceName])
	if shouldUpdate {
		dm.devices[resourceName] = newDevices
	}

	return shouldUpdate
}
