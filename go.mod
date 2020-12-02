module github.com/networkservicemesh/cmd-forwarder-sriov

go 1.15

require (
	github.com/antonfisher/nested-logrus-formatter v1.1.0
	github.com/edwarnicke/exechelper v1.0.2
	github.com/edwarnicke/grpcfd v0.0.0-20200920223154-d5b6e1f19bd0
	github.com/fsnotify/fsnotify v1.4.9
	github.com/golang/protobuf v1.4.3
	github.com/google/uuid v1.1.2
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/networkservicemesh/api v0.0.0-20201117093615-ae6039374f31
	github.com/networkservicemesh/sdk v0.0.0-20201201111053-98deb86e4ee8
	github.com/networkservicemesh/sdk-kernel v0.0.0-20201201111430-ba4b8fa9fcfc
	github.com/networkservicemesh/sdk-sriov v0.0.0-20201201111831-dcfb37900a19
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.7.0
	github.com/spiffe/go-spiffe/v2 v2.0.0-alpha.4.0.20200528145730-dc11d0c74e85
	github.com/stretchr/testify v1.6.1
	google.golang.org/grpc v1.33.2
	k8s.io/kubelet v0.20.0-beta.2
)
