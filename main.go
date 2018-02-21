package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/github/kube-service-exporter/pkg/controller"
	capi "github.com/hashicorp/consul/api"
	"github.com/spf13/viper"
)

func main() {
	viper.SetEnvPrefix("KSE")
	viper.AutomaticEnv()
	viper.SetDefault("CONSUL_KV_PREFIX", "kube-service-exporter")
	viper.SetDefault("CONSUL_HOST", "127.0.0.1")
	viper.SetDefault("CONSUL_PORT", 8500)

	namespaces := viper.GetStringSlice("NAMESPACE_LIST")
	clusterId := viper.GetString("CLUSTER_ID")
	kvPrefix := viper.GetString("CONSUL_KV_PREFIX")
	consulHost := viper.GetString("CONSUL_HOST")
	consulPort := viper.GetInt("CONSUL_PORT")

	log.Printf("Watching the following namespaces: %v", namespaces)

	stoppedC := make(chan struct{})

	ic, err := controller.NewInformerConfig()
	if err != nil {
		log.Fatal(err)
	}

	consulCfg := capi.DefaultConfig()
	consulCfg.Address = fmt.Sprintf("%s:%d", consulHost, consulPort)
	target, err := controller.NewConsulTarget(consulCfg, kvPrefix, clusterId)
	if err != nil {
		log.Fatal(err)
	}

	sw := controller.NewServiceWatcher(ic, namespaces, clusterId, target)
	go func() {
		if err := target.StartElector(); err != nil {
			log.Fatal(err)
		}
	}()
	go sw.Run()

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM)
	<-sigC
	log.Println("Shutting down...")

	go func() {
		defer close(stoppedC)
		sw.Stop()
		log.Println("Stopped Service Watcher.")
		target.StopElector()
		log.Println("Stopped Consul leadership elector.")
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
