module github.com/networkservicemesh/cmd-forwarder-sriov

go 1.16

require (
	github.com/antonfisher/nested-logrus-formatter v1.1.0
	github.com/edwarnicke/exechelper v1.0.2
	github.com/edwarnicke/grpcfd v0.1.0
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/networkservicemesh/api v0.5.1-0.20210618123026-2eb031b7db63
	github.com/networkservicemesh/sdk v0.5.1-0.20210618123609-379badf2bfa7
	github.com/networkservicemesh/sdk-k8s v0.0.0-20210618124046-6b6a6bf270f6
	github.com/networkservicemesh/sdk-sriov v0.0.0-20210618124636-09f7219c985a
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.7.0
	github.com/spiffe/go-spiffe/v2 v2.0.0-alpha.4.0.20200528145730-dc11d0c74e85
	github.com/stretchr/testify v1.7.0
	google.golang.org/grpc v1.35.0
	k8s.io/kubelet v0.20.1
)
