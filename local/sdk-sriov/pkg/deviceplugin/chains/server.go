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

// Package chains provide a chain implementation of pluginapi.DevicePluginServer
package chains

import (
	"context"
	"fmt"

	"github.com/imdario/mergo"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/deviceplugin"
)

// DevicePluginServer is a chain element interface for DevicePluginServerChain
type DevicePluginServer interface {
	// GetDevicePluginOptions returns options to be communicated with Device
	// Manager
	GetDevicePluginOptions(ctx context.Context) (*pluginapi.DevicePluginOptions, error)
	// ListAndWatch returns a stream of List of Devices
	// Whenever a Device state change or a Device disappears, ListAndWatch
	// returns the new list
	ListAndWatch(receiver chan<- *pluginapi.ListAndWatchResponse) error
	// Allocate is called during container creation so that the Device
	// Plugin can run device specific operations and instruct Kubelet
	// of the steps to make the Device available in the container
	Allocate(ctx context.Context, request *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error)
	// PreStartContainer is called, if indicated by Device Plugin during registration phase,
	// before each container start. Device plugin can run device specific operations
	// such as resetting the device before making devices available to the container
	PreStartContainer(ctx context.Context, request *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error)
}

// DevicePluginResourceServer is a chain element defining the resource name
type DevicePluginResourceServer interface {
	// ResourceName returns the resource name
	ResourceName() string

	DevicePluginServer
}

// DevicePluginServerChain is a chain implementation of pluginapi.DevicePluginServer
type DevicePluginServerChain interface {
	// Start starts server with the given context
	Start(ctx context.Context) error
	// Stop stops server
	Stop() error

	pluginapi.DevicePluginServer
}

type serverChain struct {
	resourceName     string
	servers          []DevicePluginServer
	devicesByServers map[DevicePluginServer][]*pluginapi.Device
	stop             context.CancelFunc
}

type serverListAndWatchResponse struct {
	server  DevicePluginServer
	devices []*pluginapi.Device
}

// NewDevicePluginServerChain creates a new DevicePluginServer chain from the given servers
func NewDevicePluginServerChain(resourceServer DevicePluginResourceServer, servers ...DevicePluginServer) DevicePluginServerChain {
	return &serverChain{
		resourceName:     resourceServer.ResourceName(),
		servers:          append(servers, resourceServer),
		devicesByServers: map[DevicePluginServer][]*pluginapi.Device{},
	}
}

func (sc *serverChain) Start(parentCtx context.Context) error {
	var ctx context.Context
	ctx, sc.stop = context.WithCancel(parentCtx)

	socket, err := deviceplugin.StartDeviceServer(ctx, sc)
	if err != nil {
		return err
	}

	options, err := sc.GetDevicePluginOptions(ctx, &pluginapi.Empty{})
	if err != nil {
		return err
	}

	if err := deviceplugin.RegisterDeviceServer(ctx, &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     socket,
		ResourceName: sc.resourceName,
		Options:      options,
	}); err != nil {
		return err
	}

	return nil
}

func (sc *serverChain) Stop() error {
	if sc.stop == nil {
		return fmt.Errorf("server is not running")
	}
	sc.stop()
	return nil
}

func (sc *serverChain) GetDevicePluginOptions(ctx context.Context, _ *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	options := &pluginapi.DevicePluginOptions{}
	for i := range sc.servers {
		opts, err := sc.servers[i].GetDevicePluginOptions(ctx)
		if err != nil {
			return nil, err
		}
		if opts.PreStartRequired {
			options.PreStartRequired = true
		}
	}
	return options, nil
}

func (sc *serverChain) ListAndWatch(_ *pluginapi.Empty, server pluginapi.DevicePlugin_ListAndWatchServer) error {
	response := &pluginapi.ListAndWatchResponse{}

	respCh := make(chan *serverListAndWatchResponse)
	for i := range sc.servers {
		server := sc.servers[i]
		if err := server.ListAndWatch(wrapResponseChannel(respCh, server)); err != nil {
			return err
		}
	}

	go func() {
		for {
			resp := <-respCh
			if resp == nil {
				return
			}
			response.Devices = sc.updateDevices(resp.server, resp.devices)
			if err := server.Send(response); err != nil {
				close(respCh)
				return
			}
		}
	}()
	return nil
}

func wrapResponseChannel(respCh chan *serverListAndWatchResponse, server DevicePluginServer) chan *pluginapi.ListAndWatchResponse {
	serverRespCh := make(chan *pluginapi.ListAndWatchResponse)
	go func() {
		for {
			resp := <-serverRespCh
			if resp == nil {
				close(respCh)
				return
			}
			respCh <- &serverListAndWatchResponse{
				server:  server,
				devices: resp.Devices,
			}
		}
	}()
	return serverRespCh
}

func (sc *serverChain) updateDevices(server DevicePluginServer, serverDevices []*pluginapi.Device) []*pluginapi.Device {
	sc.devicesByServers[server] = serverDevices

	var devices []*pluginapi.Device
	for _, servDevices := range sc.devicesByServers {
		devices = append(devices, servDevices...)
	}
	return devices
}

func (sc *serverChain) Allocate(ctx context.Context, request *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	response := &pluginapi.AllocateResponse{
		ContainerResponses: make([]*pluginapi.ContainerAllocateResponse, len(request.ContainerRequests)),
	}
	for i := range response.ContainerResponses {
		response.ContainerResponses[i] = &pluginapi.ContainerAllocateResponse{}
	}

	for i := range sc.servers {
		resp, err := sc.servers[i].Allocate(ctx, request)
		if err != nil {
			return nil, err
		}
		for k := range response.ContainerResponses {
			_ = mergo.Merge(response.ContainerResponses[k], resp.ContainerResponses[k], mergo.WithAppendSlice)
		}
	}

	return response, nil
}

func (sc *serverChain) PreStartContainer(ctx context.Context, request *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	for i := range sc.servers {
		if _, err := sc.servers[i].PreStartContainer(ctx, request); err != nil {
			return nil, err
		}
	}
	return &pluginapi.PreStartContainerResponse{}, nil
}
