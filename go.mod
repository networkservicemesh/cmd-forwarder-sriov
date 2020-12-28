module github.com/networkservicemesh/cmd-forwarder-sriov

go 1.15

require (
	github.com/antonfisher/nested-logrus-formatter v1.1.0
	github.com/edwarnicke/exechelper v1.0.2
	github.com/edwarnicke/grpcfd v0.0.0-20200920223154-d5b6e1f19bd0
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/networkservicemesh/api v0.0.0-20201204203731-4294f67deaa4
	github.com/networkservicemesh/sdk v0.0.0-20201228071212-4e3006826a6a
	github.com/networkservicemesh/sdk-k8s v0.0.0-20201215144953-5c91828f35ea
	github.com/networkservicemesh/sdk-sriov v0.0.0-20201214115506-af9f4fd3f099
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.7.0
	github.com/spiffe/go-spiffe/v2 v2.0.0-alpha.4.0.20200528145730-dc11d0c74e85
	github.com/stretchr/testify v1.6.1
	google.golang.org/grpc v1.33.2
	k8s.io/kubelet v0.20.1
)

replace (
	github.com/networkservicemesh/sdk-k8s => github.com/Bolodya1997/sdk-k8s v0.0.0-20201229032818-b04374a863b8
	github.com/networkservicemesh/sdk-sriov => github.com/Bolodya1997/sdk-sriov v0.0.0-20201228124236-54f053b57119
)
