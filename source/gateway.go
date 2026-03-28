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
	"log"
	"strings"

	"github.com/blake/external-mdns/resource"
	"github.com/jpillora/go-tld"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayinformers "sigs.k8s.io/gateway-api/pkg/client/informers/externalversions"
)

// GatewaySource handles adding, updating, or removing mDNS record advertisements for Gateway resources
type GatewaySource struct {
	namespace      string
	notifyChan     chan<- resource.Resource
	sharedInformer cache.SharedIndexInformer
}

// Run starts shared informers and waits for the shared informer cache to
// synchronize.
func (g *GatewaySource) Run(stopCh chan struct{}) error {
	g.sharedInformer.Run(stopCh)
	if !cache.WaitForCacheSync(stopCh, g.sharedInformer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
	}
	return nil
}

func (g *GatewaySource) onAdd(obj interface{}) {
	advertiseRecords, err := g.buildRecords(obj, resource.Added)

	if err != nil {
		fmt.Println("Error adding ingress")
		return
	}

	for _, record := range advertiseRecords {
		g.notifyChan <- record
	}
}

func (g *GatewaySource) onDelete(obj interface{}) {
	advertiseRecords, err := g.buildRecords(obj, resource.Deleted)

	if err != nil {
		fmt.Println("Error deleting ingress")
		return
	}

	for _, record := range advertiseRecords {
		g.notifyChan <- record
	}
}

func (g *GatewaySource) onUpdate(oldObj interface{}, newObj interface{}) {
	oldResources, err1 := g.buildRecords(oldObj, resource.Updated)
	if err1 != nil {
		fmt.Printf("Error gathering old ingress resources: %s", err1)
	}

	for _, record := range oldResources {
		record.Action = resource.Deleted
		g.notifyChan <- record
	}

	newResources, err2 := g.buildRecords(newObj, resource.Updated)
	if err2 != nil {
		fmt.Printf("Error gathering new ingress resources: %s", err2)
	}

	for _, record := range newResources {
		record.Action = resource.Added
		g.notifyChan <- record
	}
}

func (g *GatewaySource) buildRecords(obj any, action string) ([]resource.Resource, error) {
	var records []resource.Resource

	gateway, ok := obj.(*v1.Gateway)
	if !ok {
		return records, nil
	}

	var ipFields []string
	for _, address := range gateway.Status.Addresses {
		if *address.Type == v1.IPAddressType {
			ipFields = append(ipFields, address.Value)
		}
	}

	if len(ipFields) == 0 {
		return records, nil
	}

	// Advertise each hostname under this Ingress
	var hostname string
	for _, listener := range gateway.Spec.Listeners {
		// Skip rules with no hostname
		if listener.Hostname == nil {
			continue
		}
		fakeURL := fmt.Sprintf("http://%s", string(*listener.Hostname))
		// Skip rules that do not use the .local TLD
		if !strings.HasSuffix(fakeURL, ".local") {
			continue
		}

		parsedHost, err := tld.Parse(fakeURL)
		if err != nil {
			log.Printf("Unable to parse hostname %s. %s", string(*listener.Hostname), err.Error())
			continue
		}

		if parsedHost.Subdomain != "" {
			hostname = fmt.Sprintf("%s.%s", parsedHost.Subdomain, parsedHost.Domain)
		} else {
			hostname = parsedHost.Domain
		}
		advertiseObj := resource.Resource{
			SourceType: "gateway",
			Action:     action,
			Names:      []string{hostname},
			Namespace:  gateway.Namespace,
			IPs:        ipFields,
		}

		records = append(records, advertiseObj)
	}
	return records, nil
}

// NewIngressWatcher creates an IngressSource
func NewGatewayWatcher(factory gatewayinformers.SharedInformerFactory, namespace string, notifyChan chan<- resource.Resource) GatewaySource {
	gatewayInformer := factory.Gateway().V1().Gateways().Informer()
	g := &GatewaySource{
		namespace:      namespace,
		notifyChan:     notifyChan,
		sharedInformer: gatewayInformer,
	}

	_, err := gatewayInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    g.onAdd,
		DeleteFunc: g.onDelete,
		UpdateFunc: g.onUpdate,
	})
	if err != nil {
		log.Printf("Add Gateway watcher error %s\n", err)
	}

	return *g
}
