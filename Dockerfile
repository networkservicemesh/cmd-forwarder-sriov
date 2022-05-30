FROM golang:1.18.2-buster as go
ENV GO111MODULE=on
ENV CGO_ENABLED=0
ENV GOBIN=/bin
RUN go install github.com/go-delve/delve/cmd/dlv@v1.8.2
RUN go install github.com/grpc-ecosystem/grpc-health-probe@v0.4.1
ADD https://github.com/spiffe/spire/releases/download/v1.2.2/spire-1.2.2-linux-x86_64-glibc.tar.gz .
RUN tar xzvf spire-1.2.2-linux-x86_64-glibc.tar.gz -C /bin --strip=2 spire-1.2.2/bin/spire-server spire-1.2.2/bin/spire-agent

FROM go as build
WORKDIR /build
COPY go.mod go.sum ./
COPY ./internal/imports ./internal/imports
RUN go build ./internal/imports
COPY . .
RUN go build -o /bin/forwarder .

FROM build as test
ENV NSM_DEVICE_PLUGIN_PATH=/build/run/device-plugins
RUN mkdir -p ${NSM_DEVICE_PLUGIN_PATH}
ENV NSM_POD_RESOURCES_PATH=/build/run/pod-resources
RUN mkdir -p ${NSM_POD_RESOURCES_PATH}
ENV NSM_SRIOV_CONFIG_FILE=/dev/null
CMD go test -test.v ./...

FROM test as debug
CMD dlv -l :40000 --headless=true --api-version=2 test -test.v ./...

FROM alpine as runtime
COPY --from=build /bin/forwarder /bin/forwarder
COPY --from=build /bin/dlv /bin/dlv
ENTRYPOINT ["/bin/forwarder"]
