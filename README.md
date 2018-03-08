# kube-service-exporter

## Overview

Traffic ingress into a cluster from on-prem or custom load balancers is complex. Commonly, Ingress controllers are used to support this use case, but in many cases, this creates an unnecessary intermediate proxy and support for TCP ingress is still poor. `kube-service-exporter` is a Kubernetes controller that exports metadata about Kubernetes Services to an alternative data store which can be used to configure a custom load balancer according to individual needs.  Additional metadata about the configuration is stored in Annotations on the Service which mirror the intent of Service Annotations for configuring AWS load balancers.

Any Service which is of `type LoadBalancer` will be exported.

Presently the only export target is Consul, but the target could be any data store.

## Annotations

The following Annotations are supported. Note that these Annotations only describe *intent* and their implementation is specific to the export target (e.g. Consul) and the consumer of the configuration (e.g. `haproxy` configured via `consul-template`). As with all Kubernetes Annotations, the values should be strings.

* `kube-service-exporter.github.com/load-balancer-proxy-protocol` - Set to `"*"` to signal that all backends support PROXY Protocol.
* `kube-service-exporter.github.com/load-balancer-class` - The load balancer class is load balancer that the service should be a member of.  Examples might include "internal", "public", "corp", etc. It is up to the consumer to decide how this field is used.
* `kube-service-exporter.github.com/load-balancer-backend-protocol` - This is used to specify the protocol spoken by the backend (pod) behind a listener. Options are `http` or `tcp` for HTTP backends or TCP backends.
* `kube-service-exporter.github.com/load-balancer-health-check-path` - A path or URI for an HTTP health check
* `kube-service-exporter.github.com/load-balancer-health-check-port` - The port for the Health check. If unset, defaults to the Service NodePort.
* `kube-service-exporter.github.com/load-balancer-service-per-cluster` - If unset (or set to `"false"`), this will create a separately named service *per cluster id*.  This is useful for applications that should *not* be load balanced across multiple clusters.  The default is `"true"`, which will aggregate the same service across different clusters into the same name.  Service uniqueness is defined by a tuple of namespace, name, & port name with an optional cluster id.
* `kube-service-exporter.github.com/load-balancer-dns-name` - The DNS name that should be routed to this Service.


## Consul Target

### Operation

