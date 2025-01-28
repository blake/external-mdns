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

package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"

	"github.com/blake/external-mdns/mdns"
	"github.com/blake/external-mdns/resource"
	"github.com/blake/external-mdns/source"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
)

type k8sSource []string

func (s *k8sSource) String() string {
	return fmt.Sprint(*s)
}

func (s *k8sSource) Set(value string) error {
	switch value {
	case "ingress", "service":
		*s = append(*s, value)
	}
	return nil
}

/*
The following functions were obtained from
https://www.gmarik.info/blog/2019/12-factor-golang-flag-package/

	- getConfig()
	- lookupEnvOrInt()
	- lookupEnvOrString()
*/

func getConfig(fs *flag.FlagSet) []string {
	cfg := make([]string, 0, 10)
	fs.VisitAll(func(f *flag.Flag) {
		cfg = append(cfg, fmt.Sprintf("%s:%q", f.Name, f.Value.String()))
	})

	return cfg
}

func lookupEnvOrString(key string, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func lookupEnvOrInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok {
		v, err := strconv.Atoi(val)
		if err != nil {
			log.Fatalf("lookupEnvOrInt[%s]: %v", key, err)
		}
		return v
	}
	return defaultVal
}

func lookupEnvOrBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		v, err := strconv.ParseBool(val)
		if err != nil {
			log.Fatalf("lookupEnvOrBool[%s]: %v", key, err)
		}
		return v
	}
	return defaultVal
}

/*
The following functions were obtained from
https://gist.github.com/trajber/7cb6abd66d39662526df

  - hexDigit
  - reverseAddress()
*/
const hexDigit = "0123456789abcdef"

func reverseAddress(addr string) (arpa string, err error) {
	ip := net.ParseIP(addr)
	if ip == nil {
		return "", &net.DNSError{Err: "unrecognized address", Name: addr}
	}
	if ip.To4() != nil {
		return net.IPv4(ip[15], ip[14], ip[13], ip[12]).String() + ".in-addr.arpa.", nil
	}
	// Must be IPv6
	buf := make([]byte, 0, len(ip)*4+len("ip6.arpa."))
	// Add it, in reverse, to the buffer
	for i := len(ip) - 1; i >= 0; i-- {
		v := ip[i]
		buf = append(buf, hexDigit[v&0xF])
		buf = append(buf, '.')
		buf = append(buf, hexDigit[v>>4])
		buf = append(buf, '.')
	}
	// Append "ip6.arpa." and return (buf already has the final .)
	buf = append(buf, "ip6.arpa."...)
	return string(buf), nil
}

var (
	master           		= ""
	namespace        		= ""
	defaultNamespace 		= "default"
	withoutNamespace 		= false
	test             		= flag.Bool("test", false, "testing mode, no connection to k8s")
	sourceFlag       		k8sSource
	kubeconfig       		string
	exposeIPv4       		= true
	exposeIPv6       		= false
	publishInternal  		= flag.Bool("publish-internal-services", false, "Publish DNS records for ClusterIP services (optional)")
	recordTTL        		= 120
	truncateLongRecords 		bool
)

func constructRecords(r resource.Resource) []string {
	var records []string

	for _, resourceIP := range r.IPs {
		ip := net.ParseIP(resourceIP)
		if ip == nil {
			continue
		}

		reverseIP, _ := reverseAddress(resourceIP)

		var recordType string
		if ip.To4() != nil {
			if !exposeIPv4 {
				continue
			}
			recordType = "A"
		} else {
			if !exposeIPv6 {
				continue
			}
			recordType = "AAAA"
		}

		// Publish records resources as <name>.<namespace>.local
		// Ensure corresponding PTR records map to this hostname
		records = append(records, validatedRecord(r.Name, r.Namespace, recordTTL, recordType, ip, truncateLongRecords))

		if reverseIP != "" {
			records = append(records, validatedPTRRecord(reverseIP, recordTTL, r.Name, r.Namespace, truncateLongRecords))
		}

		// Publish records resources as <name>-<namespace>.local
		// Because Windows does not support subdomains resolution via mDNS and uses regular DNS query instead.
		records = append(records, validatedRecord(fmt.Sprintf("%s-%s", r.Name, r.Namespace), "", recordTTL, recordType, ip, truncateLongRecords))

		if reverseIP != "" {
			records = append(records, validatedPTRRecord(reverseIP, recordTTL, fmt.Sprintf("%s-%s", r.Name, r.Namespace), "", truncateLongRecords))
		}

		// Publish services without the name in the namespace if any of the following
		// criteria is satisfied:
		// 1. The Service exists in the default namespace
		// 2. The -without-namespace flag is equal to true
		// 3. The record to be published is from an Ingress with a defined hostname
		if r.Namespace == defaultNamespace || withoutNamespace || r.SourceType == "ingress" {
			records = append(records, validatedRecord(r.Name, "", recordTTL, recordType, ip, truncateLongRecords))

			if reverseIP != "" {
				records = append(records, validatedPTRRecord(reverseIP, recordTTL, r.Name, "", truncateLongRecords))
			}
		}
	}

	return records
}

func publishRecord(rr string) {
	if err := mdns.Publish(rr); err != nil {
		log.Fatalf(`🔥 Failed to publish record "%s": %v`, rr, err)
	}
}

func unpublishRecord(rr string) {
	if err := mdns.UnPublish(rr); err != nil {
		log.Fatalf(`🔥 Failed to unpublish record "%s": %v`, rr, err)
	}
}

func main() {

	// Kubernetes options
	flag.StringVar(&kubeconfig, "kubeconfig", lookupEnvOrString("EXTERNAL_MDNS_KUBECONFIG", kubeconfigPath()), "(optional) Absolute path to the kubeconfig file")
	flag.StringVar(&master, "master", lookupEnvOrString("EXTERNAL_MDNS_MASTER", master), "URL to Kubernetes master")

	// External-mDNS options
	flag.StringVar(&defaultNamespace, "default-namespace", lookupEnvOrString("EXTERNAL_MDNS_DEFAULT_NAMESPACE", defaultNamespace), "Namespace in which services should also be published with a shorter entry")
	flag.BoolVar(&withoutNamespace, "without-namespace", lookupEnvOrBool("EXTERNAL_MDNS_WITHOUT_NAMESPACE", withoutNamespace), "Published with a shorter entry without namespace (default: false)")
	flag.StringVar(&namespace, "namespace", lookupEnvOrString("EXTERNAL_MDNS_NAMESPACE", namespace), "Limit sources of endpoints to a specific namespace (default: all namespaces)")
	flag.Var(&sourceFlag, "source", "The resource types that are queried for endpoints; specify multiple times for multiple sources (required, options: service, ingress)")
	flag.BoolVar(&exposeIPv4, "expose-ipv4", lookupEnvOrBool("EXTERNAL_MDNS_EXPOSE_IPV4", exposeIPv4), "Publish A DNS entry (default: true)")
	flag.BoolVar(&exposeIPv6, "expose-ipv6", lookupEnvOrBool("EXTERNAL_MDNS_EXPOSE_IPV6", exposeIPv6), "Publish AAAA DNS entry (default: false)")
	flag.IntVar(&recordTTL, "record-ttl", lookupEnvOrInt("EXTERNAL_MDNS_RECORD_TTL", recordTTL), "DNS record time-to-live")
	flag.BoolVar(&truncateLongRecords, "truncate-long-records", lookupEnvOrBool("EXTERNAL_MDNS_TRUNCATE_LONG_RECORDS", false), "Truncate long record names using SHA-256 hash (default: false)")

	flag.Parse()

	if *test {
		publishRecord("router.local. 60 IN A 192.168.1.254")
		publishRecord("254.1.168.192.in-addr.arpa. 60 IN PTR router.local.")

		select {}
	}

	// No sources provided.
	if len(sourceFlag) == 0 {
		log.Println("❌ Error: No sources specified. Please specify at least one source to sync records from")
		os.Exit(1)
	}

	// Print parsed configuration
	log.Printf("🚀 Starting external-mdns with configuration:\n%v\n", getConfig(flag.CommandLine))

	k8sClient, err := newK8sClient()
	if err != nil {
		log.Fatalf("🔥 Failed to create Kubernetes client: %v", err)
	}

	notifyMdns := make(chan resource.Resource)
	stopper := make(chan struct{})
	defer close(stopper)
	defer runtime.HandleCrash()

	factory := informers.NewSharedInformerFactory(k8sClient, 0)
	for _, src := range sourceFlag {
		switch src {
		case "ingress":
			ingressController := source.NewIngressWatcher(factory, namespace, notifyMdns)
			go ingressController.Run(stopper)
		case "service":
			serviceController := source.NewServicesWatcher(factory, namespace, notifyMdns, publishInternal)
			go serviceController.Run(stopper)
		}
	}

	for {
		select {
		case advertiseResource := <-notifyMdns:
			for _, record := range constructRecords(advertiseResource) {
				if record == "" {
					continue
				}
				switch advertiseResource.Action {
				case resource.Added:
					log.Printf("➕ Publishing new DNS record: %s\n", record)
					publishRecord(record)
				case resource.Deleted:
					log.Printf("➖ Removing DNS record: %s\n", record)
					unpublishRecord(record)
				}
			}
		case <-stopper:
			log.Println("🛑 Stopping external-mdns")
		}
	}
}
