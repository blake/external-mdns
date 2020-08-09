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
	"path/filepath"
	"strconv"

	"github.com/blake/external-mdns/sources"
	externalMDNSTypes "github.com/blake/external-mdns/types"
	"github.com/flix-tech/k8s-mdns/mdns"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

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

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}

	// Windows
	return os.Getenv("USERPROFILE")
}

type source []string

func (s *source) String() string {
	return fmt.Sprint(*s)
}

func (s *source) Set(value string) error {
	switch value {
	case "ingress", "service":
		*s = append(*s, value)
	}
	return nil
}

func mustPublish(rr string) {
	if err := mdns.Publish(rr); err != nil {
		log.Fatalf(`Unable to publish record "%s": %v`, rr, err)
	}
}

func mustUnPublish(rr string) {
	if err := mdns.UnPublish(rr); err != nil {
		log.Fatalf(`Unable to publish record "%s": %v`, rr, err)
	}
}

var (
	master            = ""
	namespace         = ""
	defaultNamespace  = "default"
	test              = flag.Bool("test", false, "testing mode, no connection to k8s")
	sourceFlag        source
	kubeconfigDefault string
	kubeconfig        string
	publishInternal   = flag.Bool("publish-internal-services", false, "Publish DNS records for ClusterIP services (optional)")
	recordTTL         = 120
)

func main() {
	if home := homeDir(); home != "" {
		kubeconfigDefault = filepath.Join(home, ".kube", "config")
	} else {
		kubeconfigDefault = ""
	}

	// Kubernetes options
	flag.StringVar(&kubeconfig, "kubeconfig", lookupEnvOrString("EXTERNAL_MDNS_KUBECONFIG", kubeconfigDefault), "(optional) Absolute path to the kubeconfig file")
	flag.StringVar(&master, "master", lookupEnvOrString("EXTERNAL_MDNS_MASTER", master), "URL to Kubernetes master")

	// External-mDNS options
	flag.StringVar(&defaultNamespace, "default-namespace", lookupEnvOrString("EXTERNAL_MDNS_DEFAULT_NAMESPACE", defaultNamespace), "Namespace in which services should also be published with a shorter entry")
	flag.StringVar(&namespace, "namespace", lookupEnvOrString("EXTERNAL_MDNS_NAMESPACE", namespace), "Limit sources of endpoints to a specific namespace (default: all namespaces)")
	flag.Var(&sourceFlag, "source", "The resource types that are queried for endpoints; specify multiple times for multiple sources (required, options: service, ingress)")
	flag.IntVar(&recordTTL, "record-ttl", lookupEnvOrInt("EXTERNAL_MDNS_RECORD_TTL", recordTTL), "DNS record time-to-live")

	flag.Parse()

	if *test {
		mustPublish("router.local. 60 IN A 192.168.1.254")
		mustPublish("254.1.168.192.in-addr.arpa. 60 IN PTR router.local.")

		select {}
	}

	// No sources provided.
	if len(sourceFlag) == 0 {
		fmt.Println("Specify at least once source to sync records from.")
		os.Exit(1)
	}

	// Print parsed configuration
	log.Printf("app.config %v\n", getConfig(flag.CommandLine))

	// Use Kubernetes service account for authentication when running in-cluster
	var config *rest.Config
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); !os.IsNotExist(err) {
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
		// Use kubeconfig for authentication when running out-of-cluster
	} else {
		// Uses the current context in kubeconfig
		config, err = clientcmd.BuildConfigFromFlags(master, kubeconfig)
		if err != nil {
			log.Fatalln("Failed to read kubeconfig:", err)
		}
	}

	// creates the k8sClient
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalln("Failed to create Kubernetes client:", err)
	}

	resources := make(chan externalMDNSTypes.Resource)

	for _, src := range sourceFlag {
		switch src {
		case "ingress":
			go sources.WatchIngresses(k8sClient, namespace, resources)
		case "service":
			go sources.WatchServices(k8sClient, namespace, resources, publishInternal)
		}
	}

	log.Println("Waiting for resources to publish...")
	for {
		advertiseResource := <-resources

		ip := net.ParseIP(advertiseResource.IP)
		if ip == nil {
			continue
		}

		// Construct reverse IP
		reverseIP := net.IPv4(ip[15], ip[14], ip[13], ip[12])

		records := []string{
			fmt.Sprintf("%s.%s.local. %d IN A %s", advertiseResource.Name, advertiseResource.Namespace, recordTTL, ip),
			fmt.Sprintf("%s.in-addr.arpa. %d IN PTR %s.%s.local.", reverseIP, recordTTL, advertiseResource.Name, advertiseResource.Namespace),
		}

		if advertiseResource.Namespace == defaultNamespace {
			records = append(records, fmt.Sprintf("%s.local. %d IN A %s", advertiseResource.Name, recordTTL, ip))
		}

		switch advertiseResource.Action {
		case watch.Added:
			for _, record := range records {
				log.Printf("Added %s\n", record)
				mustPublish(record)
			}
		case watch.Deleted:
			for _, record := range records {
				log.Printf("Remove %s\n", record)
				mustUnPublish(record)
			}
		}
	}
}
