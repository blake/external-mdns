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
	"fmt"
	"log"
	"net"

	"github.com/blake/external-mdns/resource"
	"github.com/flix-tech/k8s-mdns/mdns"
)

func constructRecords(r resource.Resource) []string {
	var records []string

	ip := net.ParseIP(r.IP)
	if ip == nil {
		return records
	}

	// Construct reverse IP
	reverseIP := net.IPv4(ip[15], ip[14], ip[13], ip[12])

	records = append(records, fmt.Sprintf("%s.%s.local. %d IN A %s", r.Name, r.Namespace, recordTTL, ip))
	records = append(records, fmt.Sprintf("%s.in-addr.arpa. %d IN PTR %s.%s.local.", reverseIP, recordTTL, r.Name, r.Namespace))

	if r.Namespace == defaultNamespace {
		records = append(records, fmt.Sprintf("%s.local. %d IN A %s", r.Name, recordTTL, ip))
	}

	return records
}

func publishRecord(rr string) {
	if err := mdns.Publish(rr); err != nil {
		log.Fatalf(`Unable to publish record "%s": %v`, rr, err)
	}
}

func unpublishRecord(rr string) {
	if err := mdns.UnPublish(rr); err != nil {
		log.Fatalf(`Unable to publish record "%s": %v`, rr, err)
	}
}
