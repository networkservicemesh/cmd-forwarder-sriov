// Copyright (c) 2020-2021 Doc.ai and/or its affiliates.
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

//+build !windows

// Package deviceplugin provides tools for setting up device plugin server
package deviceplugin

import (
	"context"
	"sync"
	"time"

	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1alpha1"

	"github.com/networkservicemesh/sdk-k8s/pkg/tools/deviceplugin"
	"github.com/networkservicemesh/sdk-k8s/pkg/tools/podresources"
	"github.com/networkservicemesh/sdk-sriov/pkg/tools/tokens"
	"github.com/networkservicemesh/sdk/pkg/tools/logger"
)

// TokenPool is a token.Pool interface
type TokenPool interface {
	Restore(ids map[string][]string) error
	AddListener(listener func())
	Tokens() map[string]map[string]bool
	Allocate(id string) error
	Free(id string) error
}

var _ pluginapi.DevicePluginServer = (*devicePluginServer)(nil)

type devicePluginServer struct {
	lock                 sync.Mutex
	ctx                  context.Context
	name                 string
	tokenPool            TokenPool
	resourcePollTimeout  time.Duration
	updateCh             chan struct{}
	allocatedTokens      map[string]bool
	resourceListerClient podresourcesapi.PodResourcesListerClient
}

// StartServers creates new SR-IOV forwarder device plugin servers and starts them
func StartServers(
	ctx context.Context,
	tokenPool TokenPool,
	resourcePollTimeout time.Duration,
	devicePluginClient *deviceplugin.Client,
	podResourcesClient *podresources.Client,
) error {
	logEntry := logger.Log(ctx).WithField("devicePluginServer", "StartServers")

	logEntry.Info("get resource lister client")
	resourceListerClient, err := podResourcesClient.GetPodResourcesListerClient(ctx)
	if err != nil {
		logEntry.Error("failed to get resource lister client")
		return err
	}

	resp, err := resourceListerClient.List(ctx, new(podresourcesapi.ListPodResourcesRequest))
	if err != nil {
		logEntry.Errorf("resourceListerClient unavailable: %+v", err)
		return err
	}
	_ = tokenPool.Restore(respToDeviceIDs(resp))

	for name := range tokenPool.Tokens() {
		s := &devicePluginServer{
			ctx:                  ctx,
			name:                 name,
			tokenPool:            tokenPool,
			resourcePollTimeout:  resourcePollTimeout,
			updateCh:             make(chan struct{}, 1),
			allocatedTokens:      map[string]bool{},
			resourceListerClient: resourceListerClient,
		}

		tokenPool.AddListener(s.update)

		logEntry.Infof("starting server: %v", name)
		socket, err := devicePluginClient.StartDeviceServer(s.ctx, s)
		if err != nil {
			logEntry.Errorf("error starting server: %v", name)
			return err
		}

		logEntry.Infof("registering server: %s", name)
		if err := devicePluginClient.RegisterDeviceServer(s.ctx, &pluginapi.RegisterRequest{
			Version:      pluginapi.Version,
			Endpoint:     socket,
			ResourceName: name,
		}); err != nil {
			logEntry.Errorf("error registering server: %s", name)
			return err
		}

		if err := s.monitorKubeletRestart(devicePluginClient, socket); err != nil {
			logEntry.Warnf("error monitoring kubelet restart: %s %+v", name, err)
		}
	}
	return nil
}

func respToDeviceIDs(resp *podresourcesapi.ListPodResourcesResponse) map[string][]string {
	deviceIDs := map[string][]string{}
	for _, pod := range resp.PodResources {
		for _, container := range pod.Containers {
			for _, device := range container.Devices {
				deviceIDs[device.ResourceName] = append(deviceIDs[device.ResourceName], device.DeviceIds...)
			}
		}
	}
	return deviceIDs
}

func (s *devicePluginServer) update() {
	select {
	case s.updateCh <- struct{}{}:
	default:
	}
}

func (s *devicePluginServer) monitorKubeletRestart(devicePluginClient *deviceplugin.Client, socket string) error {
	logEntry := logger.Log(s.ctx).WithField("devicePluginServer", "monitorKubeletRestart")

	resetCh, err := devicePluginClient.MonitorKubeletRestart(s.ctx)
	if err != nil {
		return err
	}

	go func() {
		logEntry.Infof("start monitoring kubelet restart: %s", s.name)
		defer logEntry.Infof("stop monitoring kubelet restart: %s", s.name)
		for {
			select {
			case <-s.ctx.Done():
				return
			case _, ok := <-resetCh:
				if !ok {
					return
				}
				logEntry.Infof("re registering server: %s", s.name)
				if err = devicePluginClient.RegisterDeviceServer(s.ctx, &pluginapi.RegisterRequest{
					Version:      pluginapi.Version,
					Endpoint:     socket,
					ResourceName: s.name,
				}); err != nil {
					logEntry.Fatalf("error re registering server: %s %+v", s.name, err)
					return
				}
			}
		}
	}()

	return nil
}

func (s *devicePluginServer) GetDevicePluginOptions(_ context.Context, _ *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func (s *devicePluginServer) ListAndWatch(_ *pluginapi.Empty, server pluginapi.DevicePlugin_ListAndWatchServer) error {
	logEntry := logger.Log(s.ctx).WithField("devicePluginServer", "ListAndWatch")

	for {
		resp, err := s.resourceListerClient.List(s.ctx, new(podresourcesapi.ListPodResourcesRequest))
		if err != nil {
			logEntry.Errorf("resourceListerClient unavailable: %+v", err)
			return err
		}

		s.updateDevices(s.respToDeviceIDs(resp))

		if err := server.Send(s.listAndWatchResponse()); err != nil {
			logEntry.Errorf("server unavailable: %+v", err)
			return err
		}

		select {
		case <-s.ctx.Done():
			logEntry.Info("server stopped")
			return s.ctx.Err()
		case <-time.After(s.resourcePollTimeout):
		case <-s.updateCh:
		}
	}
}

func (s *devicePluginServer) respToDeviceIDs(resp *podresourcesapi.ListPodResourcesResponse) map[string]struct{} {
	deviceIDs := map[string]struct{}{}
	for _, pod := range resp.PodResources {
		for _, container := range pod.Containers {
			for _, device := range container.Devices {
				if device.ResourceName == s.name {
					for _, id := range device.DeviceIds {
						deviceIDs[id] = struct{}{}
					}
				}
			}
		}
	}
	return deviceIDs
}

func (s *devicePluginServer) updateDevices(allocatedIDs map[string]struct{}) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for id, allocated := range s.allocatedTokens {
		switch _, ok := allocatedIDs[id]; {
		case ok:
			s.allocatedTokens[id] = true
		case allocated:
			s.allocatedTokens[id] = false
		default:
			_ = s.tokenPool.Free(id)
			delete(s.allocatedTokens, id)
		}
	}
}

func (s *devicePluginServer) listAndWatchResponse() *pluginapi.ListAndWatchResponse {
	var devices []*pluginapi.Device
	for id, healthy := range s.tokenPool.Tokens()[s.name] {
		device := &pluginapi.Device{
			ID: id,
		}
		if healthy {
			device.Health = pluginapi.Healthy
		} else {
			device.Health = pluginapi.Unhealthy
		}
		devices = append(devices, device)
	}
	return &pluginapi.ListAndWatchResponse{
		Devices: devices,
	}
}

func (s *devicePluginServer) GetPreferredAllocation(_ context.Context, _ *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return &pluginapi.PreferredAllocationResponse{}, nil
}

func (s *devicePluginServer) Allocate(_ context.Context, request *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	resp := &pluginapi.AllocateResponse{
		ContainerResponses: make([]*pluginapi.ContainerAllocateResponse, len(request.ContainerRequests)),
	}

	var ids []string
	for i, container := range request.ContainerRequests {
		ids = append(ids, container.DevicesIDs...)

		name, value := tokens.ToEnv(s.name, container.DevicesIDs)
		resp.ContainerResponses[i] = &pluginapi.ContainerAllocateResponse{
			Envs: map[string]string{
				name: value,
			},
		}
	}

	err := s.useDevices(ids)
	s.update()

	return resp, err
}

func (s *devicePluginServer) useDevices(ids []string) (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for i := range ids {
		err = s.tokenPool.Allocate(ids[i])
		if err != nil {
			break
		}
		s.allocatedTokens[ids[i]] = true
	}

	if err != nil {
		for i := range ids {
			_ = s.tokenPool.Free(ids[i])
			delete(s.allocatedTokens, ids[i])
		}
	}

	return err
}

func (s *devicePluginServer) PreStartContainer(_ context.Context, _ *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}
