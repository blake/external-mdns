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
namespace, or `-without-namespace=true` for all namespaces.

DNS records are advertised with the format `<hostname/service_name>.<namespace>.local`.
In addition, hostnames for resources in the `-default-namespace` will also be
advertised with a short name of `<hostname/service_name>.local`.

### Additional control for Services

Service discovery is automatic, however, there are some scenarios where one may wish
to directly control names used with a more general service i.e. the service might
be in front of an Ingress Controller but the service you wish to use is not defined
with an Ingress resource such as in the case of non http/https service with nginx.

Other scenarios include non Ingress types that publish a variety of services and
act as an ingress but have configurations far more complex than can be expressed
by an Ingress resource e.g. Istio.

Additionally, one may wish to control in finer detail which services appear directly
on .local MDNS advertisements without either moving services to the default namespace
or enabling the global without-namespace flag.

In this case Service annotations are possible as follows - these annotations have
no effect if applied to an Ingress resource.

```yaml
apiVersion: v1
kind: Service
metadata:
  name: myservice
  namespace: foospace
  annotations:
    external-mdns.blakecovarrubias.com/hostnames: foo
    external-mdns.blakecovarrubias.com/without-namespace: "true"
...
spec:
  type: LoadBalancer
...
```

This example publishes the service using the name foo which will result in the names
foo.foospace.local, foo-foospace.local and, because we have specified the additional
annotation foo.local is also published (unnecessary if using the global option).

We urge you to test with the default behaviours for Services and Ingress before
using these annotations as the automatic nature of external-mdns is good enough
for most use cases.

## Deploying External-mDNS

External-mDNS is configured using argument flags. Most flags can be replaced
with environment variables. For instance, `--record-ttl` could be replaced with
`EXTERNAL_MDNS_RECORD_TTL=60`, or `--namespace kube-system` could be replaced
with `EXTERNAL_MDNS_NAMESPACE=kube-system`.

Deployment manifests are located in the [manifests/](manifests/) directory.

To deploy External-mDNS into a cluster without RBAC, use the following command.

```shell
kubectl apply --kustomize manifests/
```

To deploy External-mDNS into a cluster with RBAC, use manifests overlay.

```shell
kubectl apply --kustomize manifests/rbac
```

Verify the External-mDNS resources have correctly been deployed using
`kubectl get`.

### Without RBAC

```shell
kubectl get --kustomize manifests
```

### With RBAC

```shell
kubectl get --kustomize manifests/rbac
```

## Verifying operation

Check that External-mDNS has created the desired DNS records for your advertised
services and that they resolve to the correct load balancer or ingress IP by
using the appropriate command for your operating system.

### BSD/macOS

```console
$ dns-sd -Q example.local a in
DATE: ---Sun 16 Aug 2020---
22:50:37.797  ...STARTING...
Timestamp     A/R    Flags if Name                          Type  Class   Rdata
22:50:37.959  Add        2  4 example.local.                Addr   IN     192.0.2.10
```

### Linux

Resolve the hostname using the `getent` command.

```console
$ getent hosts example.local
192.0.2.10      example.local
```

Alternatively, you may also attempt to resolve the hostname using Avahi.

```console
$ avahi-resolve-address -4 --name example.local
example.local 192.0.2.10
```

Note about Linux DNS lookups:

If `/etc/nsswitch.conf` is configured to use the `mdns4_minimal` module,
`libnss-mdns` will reject the request if the request has more than two labels.
Example: `example.default.local` is rejected.

In order to resolve hostnames that are published from non-default Kubernetes
namespaces, modify `/etc/nsswitch.conf` and replace `mdns4_minimal` with `mdns4`.
Also, create or modify `/etc/mdns.allow` and add the following contents.

```text
# /etc/mdns.allow
.local.
.local
```

Hostnames with more than two labels should now be resolvable.

```console
$ getent hosts example.default.local
192.0.2.10      example.default.local
```

[External DNS]: https://github.com/kubernetes-sigs/external-dns
[RFC 6762]: https://tools.ietf.org/html/rfc6762
