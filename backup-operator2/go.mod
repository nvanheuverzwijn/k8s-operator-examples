module github.com/nvanheuverzwijn/backup-operator

go 1.16

require (
	github.com/aws/aws-sdk-go v1.41.16
	github.com/go-logr/logr v0.4.0 // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	k8s.io/api v0.22.3
	k8s.io/apimachinery v0.22.3
	k8s.io/cli-runtime v0.22.3
	k8s.io/client-go v0.22.3
	k8s.io/kubectl v0.22.3
	sigs.k8s.io/controller-runtime v0.10.0
)
