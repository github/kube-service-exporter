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

In `example/kube-service-exporter.yaml`, set the `KSE_CLUSTER_ID` environment variable to `value:<your-cluster-name-here>`. In our example, we simply set the cluster name to `minikube`.

```
$ kubectl apply -f examples/rbac.yaml
$ kubectl apply -f examples/kube-service-exporter.yaml
```

Check for the pods to appear in the `kube-system` namespace:

```
$ kubectl get pods -n kube-system | grep kube-service-exporter
kube-service-exporter-564dd97bcd-x6w6g      1/1     Running   3          3d
kube-service-exporter-78455495fd-fv5gm      1/1     Running   3          3d
```

### See the node and leadership exports to consul

Exec into the consul pod:
```
$ kubectl exec -it consul-64bcfb7c69-l7lb9 -- sh
/ # consul kv get -recurse
kube-service-exporter/leadership/minikube-leader:kube-service-exporter-564dd97bcd-x6w6g
kube-service-exporter/nodes/minikube:[{"Name":"minikube","Address":"10.0.2.15"}]
```

### Deploy and configure a Service

Similarly to exporting node metadata, kube-service-exporter will also export metadata about Services.

```
$ kubectl apply -f examples/service.yaml
```

Exec into the Consul pod to see the result:

```
$ kubectl exec -it consul-64bcfb7c69-l7lb9 -- sh
/ # consul kv get -recurse
kube-service-exporter/leadership/minikube-leader:kube-service-exporter-564dd97bcd-x6w6g
kube-service-exporter/nodes/minikube:[{"Name":"minikube","Address":"10.0.2.15"}]
kube-service-exporter/services/kube-system-kse-example-http/clusters/minikube:{"hash":"9d907d682bc31ae4","ClusterName":"minikube","port":32046,"dns_name":"examplecluster.example.net","health_check_path":"","health_check_port":32046,"backend_protocol":"http","proxy_protocol":false,"load_balancer_class":"internal","load_balancer_listen_port":0,"custom_attrs":{"allowed_ips":["10.0.0.0/8","172.16.0.0/12","192.168.0.0/16"]}}
```

You now can use the new key/value pair to configure your external load balancer using `consul-template` (example pending).



