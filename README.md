# External-mDNS

External-mDNS advertises exposed Kubernetes Services and Ingresses addresses on a
LAN using multicast DNS ([RFC 6762]).

It is based on <https://github.com/flix-tech/k8s-mdns/> and heavily inspired by
[External DNS].

## What It Does

External-mDNS makes Kubernetes resources discoverable on a local network via
multicast DNS without the need for a separate DNS server. It retrieves a list of
resources (Services and Ingresses) from Kubernetes and serves the record to local
clients via multicast DNS.

Hostnames associated with Ingress resources, or exposed services of type
LoadBalancer, will be advertised on the local network.

By default External-mDNS will advertise hostnames for exposed resources in all
namespaces. Use the `-namespace` flag to restrict advertisement to a single
namespace.

DNS records are advertised with the format `<hostname/service_name>.<namespace>.local`.
In addition, hostnames for resources in the `-default-namespace` will also be
advertised with a short name of `<hostname/service_name>.local`.

## Deploying External-mDNS

External-mDNS is configured using argument flags. Most flags can be replaced
with environment variables. For instance, `--record-ttl` could be replaced with
`EXTERNAL_MDNS_RECORD_TTL=60`, or `--namespace kube-system` could be replaced
with `EXTERNAL_MDNS_NAMESPACE=kube-system`.

### Manifest (without RBAC)

```yaml
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-mdns
spec:
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: external-mdns
  template:
    metadata:
      labels:
        app: external-mdns
    spec:
      hostNetwork: true
      containers:
      - name: external-mdns
        image: blakec/external-mdns:latest
        args:
        - -source=ingress
        - -source=service
```

### Manifest (with RBAC)

```yaml
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: external-mdns
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
 name: external-mdns
rules:
- apiGroups: [""]
  resources: ["services"]
  verbs: ["list", "watch"]
- apiGroups: ["extensions","networking.k8s.io"]
  resources: ["ingresses"]
  verbs: ["list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRoleBinding
metadata:
 name: external-mdns-viewer
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: external-mdns
subjects:
- kind: ServiceAccount
  name: external-mdns
  namespace: default
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-mdns
spec:
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: external-mdns
  template:
    metadata:
      labels:
        app: external-mdns
    spec:
      hostNetwork: true
      serviceAccountName: external-mdns
      containers:
      - name: external-mdns
        image: blakec/external-mdns:latest
        args:
        - -source=ingress
        - -source=service
```

Deploy External-mDNS using `kubectl apply --filename external-mdns.yaml`.

Check that External-mDNS has created the desired DNS records for your advertised
services, and that it points to its load balancer's IP.

Test that the record is resolvable from the local LAN using the appropriate
command for your operating system.

#### BSD/macOS

```console
$ dns-sd -Q example.local a in
DATE: ---Sun 16 Aug 2020---
22:50:37.797  ...STARTING...
Timestamp     A/R    Flags if Name                          Type  Class   Rdata
22:50:37.959  Add        2  4 example.local.                Addr   IN     192.0.2.10
```

#### Linux

```console
$ avahi-resolve-address -4 --name example.local
example.local 192.0.2.10
```

[External DNS]: https://github.com/kubernetes-sigs/external-dns
[RFC 6762]: https://tools.ietf.org/html/rfc6762
