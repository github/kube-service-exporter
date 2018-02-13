package main

import (
	"log"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func NewClientSet() (kubernetes.Interface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

func main() {
	namespaces := make([]string, 0, 5)

	cs, err := NewClientSet()
	if err != nil {
		log.Fatalf("Unable to acquire a clientset: %v", err)
	}

	nsList, err := cs.CoreV1().Namespaces().List(meta_v1.ListOptions{})
	if err != nil {
		log.Fatalf("Unable to list namespaces: %v", err)
	}

	for _, ns := range nsList.Items {
		namespaces = append(namespaces, ns.Name)
	}
	log.Printf("These are the namespaces: %v\n", namespaces)
}
