# Intro

This repo contains 'cmd-forwarder-sriov' a SR-IOV based forwarder application for Network Service Mesh. 

This README will provide directions for building, testing, and debugging that container.

# Usage

SR-IOV forwarder binds one of the free VFs from the list of the configured PFs for each client request. It supports 2
mechanisms for the client:
* KERNEL_INTERFACE - binds VF to the kernel driver and passes kernel interface to the client net NS
* VFIO - binds VF to the vfio-pci driver and gives permissions to the client pod to work with the VF device

Also it works as a device plugin server providing resources for the client pods. Each configured PF adds _VF count_
`serviceDomain/capalility` resources for every `{ serviceDomain, capability }` pair.

## Environment config

`cmd-forwarder-sriov` accept following environment variables:

* `NSM_NAME`                        -  A string value of forwarder network service endpoint name (default `sriov-forwarder`)
* `NSM_NSNAME`                      -  A string value of forwarder network service name (default `sriovns`)
* `NSM_CONNECT_TO`                  -  A Network Service Manager connectTo URL (default `unix:///var/lib/networkservicemesh/nsm.io.sock`)
* `NSM_MAX_TOKEN_LIFETIME`          -  A token lifetime duration (default `24h`)
* `NSM_RESOURCE_POLL_TIMEOUT`       -  A timeout to poll device plugin resources usage from kubelet API (default `30s`)
* `NSM_DEVICE_PLUGIN_PATH`          -  Path to the device plugin directory (default `/var/lib/kubelet/device-plugins/`)
* `NSM_POD_RESOURCES_PATH`          -  Path to the pod resources directory (default `/var/lib/kubelet/pod-resources/`)
* `NSM_SRIOV_CONFIG_FILE`           -  Path to the config file (default `./pci.config`)
* `NSM_PCI_DEVICES_PATH`            -  Path to the PCI devices directory (default `/sys/bus/pci/devices`)
* `NSM_PCI_DRIVERS_PATH`            -  Path to the PCI drivers directory (default `/sys/bus/pci/drivers`)
* `NSM_CGROUP_PATH`                 -  Path to the host `/sys/fs/cgroup/devices` directory (default `/host/sys/fs/cgroup/devices`)
* `NSM_VFIO_PATH`                   -  Path to the host `/dev/vfio` directory (default `/host/dev/vfio`)
* `NSM_DIAL_TIMEOUT`                -  Timeout for the dial the next endpoint
* `NSM_LABELS`                      -  Labels related to this forwarder-sriov instance
* `NSM_LOG_LEVEL`                   -  Log level
* `NSM_METRICS_EXPORT_INTERVAL`     -  interval between mertics exports
* `NSM_OPEN_TELEMETRY_ENDPOINT`     -  OpenTelemetry Collector Endpoint
* `NSM_REGISTRY_CLIENT_POLICIES`    -  paths to files and directories that contain registry client policies

## Config file

`cmd-forwarder-sriov` requires configuration file in the following format:
```yaml
---
physicalFunctions:            # map [PCI address -> PF] for all supported PFs
  0000:01:00.0:
    pfKernelDriver: pf-driver # PF kernel driver
    vfKernelDriver: vf-driver # VFs kernel driver
    capabilities:             # list of the capabilities supported by the PF
      - intel
      - 10G
    serviceDomains:           # list of the service domains supported by the PF
      - service.domain.1
```

# Build

## Build cmd binary locally

You can build the locally by executing

```bash
go build ./...
```

## Build Docker container

You can build the docker container by running:

```bash
docker build .
```

# Testing

## Testing Docker container

Testing is run via a Docker container.  To run testing run:

```bash
docker run --privileged --rm $(docker build -q --target test .)
```

# Debugging

## Debugging the tests
If you wish to debug the test code itself, that can be acheived by running:

```bash
docker run --privileged --rm -p 40000:40000 $(docker build -q --target debug .)
```

This will result in the tests running under dlv.  Connecting your debugger to localhost:40000 will allow you to debug.

```bash
-p 40000:40000
```
forwards port 40000 in the container to localhost:40000 where you can attach with your debugger.

```bash
--target debug
```

Runs the debug target, which is just like the test target, but starts tests with dlv listening on port 40000 inside the container.

## Debugging the cmd

When you run 'cmd' you will see an early line of output that tells you:

```Setting env variable DLV_LISTEN_FORWARDER to a valid dlv '--listen' value will cause the dlv debugger to execute this binary and listen as directed.```

If you follow those instructions when running the Docker container:
```bash
docker run --privileged -e DLV_LISTEN_FORWARDER=:50000 -p 50000:50000 --rm $(docker build -q --target test .)
```

```-e DLV_LISTEN_FORWARDER=:50000``` tells docker to set the environment variable DLV_LISTEN_FORWARDER to :50000 telling
dlv to listen on port 50000.

```-p 50000:50000``` tells docker to forward port 50000 in the container to port 50000 in the host.  From there, you can
just connect dlv using your favorite IDE and debug cmd.

## Debugging the tests and the cmd

```bash
docker run --privileged -e DLV_LISTEN_FORWARDER=:50000 -p 40000:40000 -p 50000:50000 --rm $(docker build -q --target debug .)
```

Please note, the tests **start** the cmd, so until you connect to port 40000 with your debugger and walk the tests
through to the point of running cmd, you will not be able to attach a debugger on port 50000 to the cmd.

## A Note on Running golangci-lint

Because cmd-forwarder-sriov is only anticipated to run in Linux, you will need to run golangci-lint run with:

```go
GOOS=linux golangci-lint run
```
