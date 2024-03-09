// Copyright 2020 Blake Covarrubias
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package source

import (
	"fmt"

	"github.com/blake/external-mdns/resource"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// ServiceSource handles adding, updating, or removing mDNS record advertisements
type ServiceSource struct {
	namespace       string
	publishInternal bool
	notifyChan      chan<- resource.Resource
	sharedInformer  cache.SharedIndexInformer
}

// Run starts shared informers and waits for the shared informer cache to
// synchronize.
func (s *ServiceSource) Run(stopCh chan struct{}) error {
	s.sharedInformer.Run(stopCh)
	if !cache.WaitForCacheSync(stopCh, s.sharedInformer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
	}
	return nil
}

func (s *ServiceSource) onAdd(obj interface{}) {
	advertiseResource, err := s.buildRecord(obj, resource.Added)

	if err != nil {
		for _, name := range advertiseResource.Names {
			fmt.Printf("updating, %s", name)
		}
	}

	if len(advertiseResource.IPs) == 0 {
		return
	}

	s.notifyChan <- advertiseResource
}

func (s *ServiceSource) onDelete(obj interface{}) {
	advertiseResource, err := s.buildRecord(obj, resource.Deleted)

	if err != nil {
		for _, name := range advertiseResource.Names {
			fmt.Printf("Error deleting, %s", name)
		}
	}
	s.notifyChan <- advertiseResource
}

func (s *ServiceSource) onUpdate(oldObj interface{}, newObj interface{}) {
	oldResource, err1 := s.buildRecord(oldObj, resource.Deleted)
	if err1 != nil {
		fmt.Printf("Error parsing old service resource: %s", err1)
	}
	s.notifyChan <- oldResource

	newResource, err2 := s.buildRecord(newObj, resource.Added)
	if err2 != nil {
		fmt.Printf("Error parsing new service resource: %s", err2)
	}
	s.notifyChan <- newResource
}

func (s *ServiceSource) buildRecord(obj interface{}, action string) (resource.Resource, error) {

	var advertiseObj = resource.Resource{
		SourceType: "service",
		Action:     action,
	}

	service, ok := obj.(*corev1.Service)

	if !ok {
		return advertiseObj, nil
	}

	advertiseObj.Names = []string{service.Name}
	advertiseObj.Namespace = service.Namespace
	advertiseObj.IPs = []string{}

	if service.Spec.Type == "ClusterIP" && s.publishInternal {
		advertiseObj.IPs = append(advertiseObj.IPs, service.Spec.ClusterIP)
	} else if service.Spec.Type == "LoadBalancer" {
		for _, lb := range service.Status.LoadBalancer.Ingress {
			if lb.IP != "" {
				advertiseObj.IPs = append(advertiseObj.IPs, lb.IP)
			}
		}
	}

	return advertiseObj, nil
}

// NewServicesWatcher creates an ServiceSource
func NewServicesWatcher(factory informers.SharedInformerFactory, namespace string, notifyChan chan<- resource.Resource, publishInternal *bool) ServiceSource {
	servicesInformer := factory.Core().V1().Services().Informer()
	s := &ServiceSource{
		namespace:       namespace,
		publishInternal: *publishInternal,
		notifyChan:      notifyChan,
		sharedInformer:  servicesInformer,
	}
	servicesInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    s.onAdd,
		DeleteFunc: s.onDelete,
		UpdateFunc: s.onUpdate,
	})

	return *s
}
