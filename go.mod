module github.com/networkservicemesh/cmd-forwarder-sriov

go 1.16

require (
	github.com/antonfisher/nested-logrus-formatter v1.3.1
	github.com/edwarnicke/exechelper v1.0.2
	github.com/edwarnicke/grpcfd v1.1.2
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/networkservicemesh/api v1.3.2-0.20220512163820-8c875d61945b
	github.com/networkservicemesh/sdk v0.5.1-0.20220513144854-3f01437e5b07
	github.com/networkservicemesh/sdk-k8s v0.0.0-20220512222121-cb9e385a3be1
	github.com/networkservicemesh/sdk-sriov v0.0.0-20220513145729-8623b140bb29
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/spiffe/go-spiffe/v2 v2.0.0
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/stretchr/testify v1.7.0
	google.golang.org/grpc v1.42.0
	k8s.io/kubelet v0.22.1
)
