# Sample Deployment of kube-service-exporter
*These instructions will demonstrate an example deployment of kube-service-exporter, and show how metadata from your cluster Nodes and Services will appear on a Consul storage*


## Prerequisites

- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
- Access to a Kubernetes cluster(this example uses [Minikube](https://github.com/kubernetes/minikube))
- [Docker](https://docs.docker.com/install/) installed on your machine

## Deploy and export to Consul

### Install consul on your cluster

```
$ kubectl apply -f examples/consul.yaml
```

Check for the pod to appear in the `kube-system` namespace:

```
$ kubectl get pods -n kube-system | grep consul
consul-64bcfb7c69-qm69n       1/1     Running   0          3d
```

### Install kube-service-exporter

In `example/kube-service-exporter.yaml`, set the `KSE_CLUSTER_ID` environment variable to `value:<your-cluster-name-here>`.

```
$ kubectl apply -f examples/rbac.yaml
$ kubectl apply -f examples/kube-service-exporter.yaml
```

Check for the pods to appear in the `kube-system` namespace:

```
$ kubectl get pods -n kube-system | grep kube-service-exporter
kube-service-exporter-78455495fd-dp4md      1/1     Running   3          3d
kube-service-exporter-78455495fd-fv5gm      1/1     Running   3          3d
```

### See the node and leadership exports to consul

Exec into the consul pod:
```
$ kubectl exec -it consul-64bcfb7c69-l7lb9 -- sh
/ # consul kv get -recurse
kube-service-exporter/leadership/democluster-leader:kube-service-exporter-78455495fd-fv5gm
kube-service-exporter/nodes/democluster:[{"Name":"minikube","Address":"10.0.2.15"}]
```

### Deploy and configure a Service

<!-- TODO: NGINX stuff here; set an annotation to something fun-->

Similarly to exporting node metadata, you can also export metadata about Services via Annotations.


