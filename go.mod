module github.com/networkservicemesh/cmd-forwarder-sriov

go 1.16

require (
	github.com/antonfisher/nested-logrus-formatter v1.1.0
	github.com/edwarnicke/exechelper v1.0.2
	github.com/edwarnicke/grpcfd v0.0.0-20210219150442-10fb469a6976
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/networkservicemesh/api v0.0.0-20210417193417-dd329f8d6b7a
	github.com/networkservicemesh/sdk v0.0.0-20210429111447-9b80b5547ab6
	github.com/networkservicemesh/sdk-k8s v0.0.0-20210429113422-5f1ac386c5cf
	github.com/networkservicemesh/sdk-sriov v0.0.0-20210429095018-d9f26fd8541a
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.7.0
	github.com/spiffe/go-spiffe/v2 v2.0.0-alpha.4.0.20200528145730-dc11d0c74e85
	github.com/stretchr/testify v1.7.0
	google.golang.org/grpc v1.35.0
	k8s.io/kubelet v0.20.1
)
