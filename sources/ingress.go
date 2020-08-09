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
	"fmt"
	"log"
	"strings"

	externalMDNSTypes "github.com/blake/external-mdns/types"
	"github.com/jpillora/go-tld"
	"k8s.io/api/extensions/v1beta1"

	k8sApiError "k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// WatchIngresses test
func WatchIngresses(clientset *kubernetes.Clientset, namespace string, resources chan<- externalMDNSTypes.Resource) {
	for {
		ingresses, err := clientset.ExtensionsV1beta1().Ingresses(namespace).Watch(context.TODO(), metaV1.ListOptions{})

		// Check if error is an authentication error
		if apiError, ok := err.(*k8sApiError.StatusError); ok {
			log.Fatalf("Error retreiving Ingress resources. Message: %s, Reason: %s", apiError.Status().Message, apiError.Status().Reason)
		} else if err != nil {
			panic(err.Error())
		}

		for {
			ev := <-ingresses.ResultChan()

			if ev.Object == nil {
				log.Fatalln("Error while watching ingresses.")
			}

			ingress := ev.Object.(*v1beta1.Ingress)

			var ipField string

			for _, lb := range ingress.Status.LoadBalancer.Ingress {
				if lb.IP != "" {
					ipField = lb.IP
				}
			}

			//  Skip if IP field is undefined
			if ipField == "" {
				continue
			}

			var hostname string
			// Advertise each hostname under this Ingress
			for _, rule := range ingress.Spec.Rules {
				// Skip rules with no hostname or that do not use the .local TLD
				if rule.Host == "" || !strings.HasSuffix(rule.Host, ".local") {
					continue
				}

				fakeURL := fmt.Sprintf("http://%s", rule.Host)
				parsedHost, err := tld.Parse(fakeURL)

				if err != nil {
					log.Printf("Unable to parse hostname %s. %s", rule.Host, err.Error())
					continue
				}

				if parsedHost.Subdomain != "" {
					hostname = fmt.Sprintf("%s.%s", parsedHost.Subdomain, parsedHost.Domain)
				} else {
					hostname = parsedHost.Domain
				}
				advertiseResource := externalMDNSTypes.Resource{
					Action:    ev.Type,
					IP:        ipField,
					Name:      hostname,
					Namespace: ingress.Namespace,
				}

				resources <- advertiseResource
			}
		}
	}
}
