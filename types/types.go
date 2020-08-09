package types

import "k8s.io/apimachinery/pkg/watch"

// Resource test
type Resource struct {
	Action    watch.EventType
	IP        string
	Name      string
	Namespace string
}
