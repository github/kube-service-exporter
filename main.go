package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/github/kube-service-exporter/pkg/controller"
	"github.com/spf13/viper"
)

func main() {
	viper.SetEnvPrefix("KSE")
	viper.AutomaticEnv()
	namespaces := viper.GetStringSlice("NAMESPACE_LIST")
	clusterId := viper.GetString("CLUSTER_ID")
	hostIP := viper.GetString("HOST_IP")

	log.Printf("Watching the following namespaces: %v", namespaces)

	stoppedC := make(chan struct{})

	ic, err := controller.NewInformerConfig()
	if err != nil {
		log.Fatal(err)
	}

	target, err := controller.NewConsulTarget(hostIP)
	if err != nil {
		log.Fatal(err)
	}
	sw := controller.NewServiceWatcher(ic, namespaces, clusterId, target)
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
