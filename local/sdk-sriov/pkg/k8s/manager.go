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

// Package k8s provides tools to access k8s API
package k8s

import (
	"context"
	"path"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	podresources "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"

	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/tools/fswatcher"
	"github.com/networkservicemesh/cmd-forwarder-sriov/local/sdk-sriov/pkg/tools/socketpath"
)

const (
	dialTimeoutDefault = 15 * time.Second
	kubeletSocket      = "kubelet.sock"
)

// Manager is a k8s API helper class
type Manager struct {
	devicePluginPath   string
	devicePluginSocket string
	podResourcesSocket string
}

// NewManager creates a new k8s manager
func NewManager(devicePluginPath, podResourcesPath string) *Manager {
	return &Manager{
		devicePluginPath:   devicePluginPath,
		devicePluginSocket: path.Join(devicePluginPath, kubeletSocket),
		podResourcesSocket: path.Join(podResourcesPath, kubeletSocket),
	}
}

// StartDeviceServer starts device plugin server and returns the name of the corresponding unix socket
func (km *Manager) StartDeviceServer(ctx context.Context, deviceServer pluginapi.DevicePluginServer) (string, error) {
	logEntry := log.Entry(ctx).WithField("Manager", "StartDeviceServer")

	socket := uuid.New().String()
	socketPath := socketpath.SocketPath(path.Join(km.devicePluginPath, socket))
	logEntry.Infof("socket = %v", socket)
	if err := socketpath.SocketCleanup(socketPath); err != nil {
		return "", err
	}

	grpcServer := grpc.NewServer()
	pluginapi.RegisterDevicePluginServer(grpcServer, deviceServer)

	socketURL := grpcutils.AddressToURL(socketPath)
	errCh := grpcutils.ListenAndServe(ctx, socketURL, grpcServer)
	go func() {
		if err := <-errCh; err != nil {
			logEntry.Errorf("error in device plugin grpc server at %v: %+v", socket, err)
		}
	}()

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeoutDefault)
	defer cancel()

	logEntry.Info("check device server operational")
	conn, err := grpc.DialContext(dialCtx, socketURL.String(), grpc.WithBlock(), grpc.WithInsecure())
	if err != nil {
		logEntry.Error(err)
		return "", err
	}
	_ = conn.Close()

	logEntry.Info("device server is operational")

	return socket, nil
}

// RegisterDeviceServer registers device plugin server using the given request
func (km *Manager) RegisterDeviceServer(ctx context.Context, request *pluginapi.RegisterRequest) error {
	logEntry := log.Entry(ctx).WithField("Manager", "RegisterDeviceServer")

	socketURL := grpcutils.AddressToURL(socketpath.SocketPath(km.devicePluginSocket))
	conn, err := grpc.DialContext(ctx, socketURL.String(), grpc.WithInsecure())
	if err != nil {
		return errors.Wrap(err, "cannot connect to device plugin kubelet service")
	}
	defer func() { _ = conn.Close() }()

	client := pluginapi.NewRegistrationClient(conn)
	logEntry.Info("trying to register to device plugin kubelet service")
	if _, err = client.Register(context.Background(), request); err != nil {
		return errors.Wrap(err, "cannot register to device plugin kubelet service")
	}
	logEntry.Info("register done")

	return nil
}

// MonitorKubeletRestart monitors if kubelet restarts so we need to re register device plugin server
func (km *Manager) MonitorKubeletRestart(ctx context.Context) (chan bool, error) {
	logEntry := log.Entry(ctx).WithField("Manager", "MonitorKubeletRestart")

	watcher, err := fswatcher.WatchOn(km.devicePluginPath)
	if err != nil {
		logEntry.Errorf("failed to watch on %v", km.devicePluginPath)
		return nil, err
	}

	monitorCh := make(chan bool, 1)
	go func() {
		defer func() { _ = watcher.Close() }()
		defer close(monitorCh)
		for {
			select {
			case <-ctx.Done():
				logEntry.Info("end monitoring")
				return
			case event, ok := <-watcher.Events:
				if !ok {
					logEntry.Info("watcher has been closed")
					return
				}
				if event.Name == km.devicePluginSocket && event.Op&fsnotify.Create == fsnotify.Create {
					logEntry.Warn("kubelet restarts")
					monitorCh <- true
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					logEntry.Info("watcher has been closed")
					return
				}
				logEntry.Warn(err)
			}
		}
	}()
	return monitorCh, nil
}

// GetPodResourcesListerClient returns a new PodResourcesListerClient
func (km *Manager) GetPodResourcesListerClient(ctx context.Context) (podresources.PodResourcesListerClient, error) {
	logEntry := log.Entry(ctx).WithField("Manager", "GetPodResourcesListerClient")

	socketURL := grpcutils.AddressToURL(socketpath.SocketPath(km.podResourcesSocket))
	conn, err := grpc.DialContext(ctx, socketURL.String(), grpc.WithInsecure())
	if err != nil {
		return nil, errors.Wrap(err, "cannot connect to pod resources kubelet service")
	}

	logEntry.Info("start pod resources client")
	go func() {
		<-ctx.Done()
		logEntry.Info("close pod resources client")
		_ = conn.Close()
	}()

	return podresources.NewPodResourcesListerClient(conn), nil
}
