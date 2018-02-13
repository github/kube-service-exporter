package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/github/kube-service-exporter/pkg/controller"

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
	stoppedC := make(chan struct{})

	cs, err := NewClientSet()
	if err != nil {
		log.Fatalf("Unable to acquire a clientset: %v", err)
	}

	sw := controller.NewServiceWatcher(cs, 15*time.Minute)
	go sw.Run()

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM)
	<-sigC
	log.Println("Shutting down...")

	go func() {
		defer close(stoppedC)
		sw.Stop()
		log.Println("Stopped Service Watcher...")
	}()

	// make sure stops don't take too long
	timer := time.NewTimer(5 * time.Second)
	select {
	case <-timer.C:
		log.Println("goroutines too long to stop. Exiting.")
	case <-stoppedC:
		log.Println("Stopped.")
	}
	os.Stdout.Sync()
}
