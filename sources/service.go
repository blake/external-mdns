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

package sources

import (
	"context"
	"log"

	externalMDNSTypes "github.com/blake/external-mdns/types"
	v1 "k8s.io/api/core/v1"
	k8sApiError "k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// WatchServices test
func WatchServices(clientset *kubernetes.Clientset, namespace string, resources chan<- externalMDNSTypes.Resource, publishInternal *bool) {
	for {
		services, err := clientset.CoreV1().Services(namespace).Watch(context.TODO(), metaV1.ListOptions{})

		// Check if error is an authentication error
		if apiError, ok := err.(*k8sApiError.StatusError); ok {
			log.Fatalf("Error retreiving Ingress resources. Message: %s, Reason: %s", apiError.Status().Message, apiError.Status().Reason)
		} else if err != nil {
			panic(err.Error())
		}

		for {
			ev := <-services.ResultChan()

			if ev.Object == nil {
				log.Fatalln("Error while watching services.")
			}

			service := ev.Object.(*v1.Service)

			var ipField string

			if service.Spec.Type == "ClusterIP" && *publishInternal {
				ipField = service.Spec.ClusterIP
			} else if service.Spec.Type == "LoadBalancer" {
				for _, lb := range service.Status.LoadBalancer.Ingress {
					if lb.IP != "" {
						ipField = lb.IP
					}
				}
			} else {
				continue
			}

			advertiseResource := externalMDNSTypes.Resource{
				Action:    ev.Type,
				Name:      service.Name,
				Namespace: service.Namespace,
				IP:        ipField,
			}

			resources <- advertiseResource
		}
	}
}
