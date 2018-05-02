package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/github/kube-service-exporter/pkg/controller"
	"github.com/github/kube-service-exporter/pkg/leader"
	"github.com/github/kube-service-exporter/pkg/server"
	"github.com/github/kube-service-exporter/pkg/stats"
	capi "github.com/hashicorp/consul/api"
	"github.com/spf13/viper"
)

func main() {
	viper.SetEnvPrefix("KSE")
	viper.AutomaticEnv()
	viper.SetDefault("CONSUL_KV_PREFIX", "kube-service-exporter")
	viper.SetDefault("CONSUL_HOST", "127.0.0.1")
	viper.SetDefault("CONSUL_PORT", 8500)
	viper.SetDefault("DOGSTATSD_HOST", "127.0.0.1")
	viper.SetDefault("DOGSTATSD_PORT", 8125)
	viper.SetDefault("HTTP_IP", "")
	viper.SetDefault("HTTP_PORT", 8080)

	namespaces := viper.GetStringSlice("NAMESPACE_LIST")
	clusterId := viper.GetString("CLUSTER_ID")
	kvPrefix := viper.GetString("CONSUL_KV_PREFIX")
	consulHost := viper.GetString("CONSUL_HOST")
	consulPort := viper.GetInt("CONSUL_PORT")
	podName := viper.GetString("POD_NAME")
	nodeSelector := viper.GetString("NODE_SELECTOR")
	dogstatsdHost := viper.GetString("DOGSTATSD_HOST")
	dogstatsdPort := viper.GetInt("DOGSTATSD_PORT")
	httpIp := viper.GetString("HTTP_IP")
	httpPort := viper.GetInt("HTTP_PORT")

	stopTimeout := 10 * time.Second

	if !viper.IsSet("CLUSTER_ID") {
		log.Fatalf("Please set the KSE_CLUSTER_ID environment variable to a unique cluster Id")
	}

	if len(namespaces) > 0 {
		log.Printf("Watching the following namespaces: %+v", namespaces)
	}

	if err := stats.Configure(dogstatsdHost, dogstatsdPort); err != nil {
		log.Fatal(err)
	}
	stats.Client().Gauge("start", 1, nil, 1)

	stoppedC := make(chan struct{})

	ic, err := controller.NewInformerConfig()
	if err != nil {
		log.Fatal(err)
	}

	nodeIC, err := controller.NewNodeInformerConfig()
	if err != nil {
		log.Fatal(err)
	}

	// Get the IP for the local consul agent since we need it in a few places
	consulIPs, err := net.LookupIP(consulHost)
	if err != nil {
		log.Fatal(err)
	}

	consulCfg := capi.DefaultConfig()
	consulCfg.Address = fmt.Sprintf("%s:%d", consulIPs[0].String(), consulPort)
	log.Printf("Using Consul agent at %s", consulCfg.Address)

	elector, err := leader.NewConsulLeaderElector(consulCfg, kvPrefix, clusterId, podName)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		if err := elector.Run(); err != nil {
			log.Fatal(err)
		}
	}()

	target, err := controller.NewConsulTarget(consulCfg, kvPrefix, clusterId, elector)
	if err != nil {
		log.Fatal(err)
	}

	sw := controller.NewServiceWatcher(ic, namespaces, clusterId, target)
	go sw.Run()

	nw := controller.NewNodeWatcher(nodeIC, target, nodeSelector)
	go nw.Run()

	httpSrv := server.New(httpIp, httpPort, stopTimeout)
	go httpSrv.Run()

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM)
	<-sigC
	log.Println("Shutting down...")
	stats.Client().Gauge("shutdown", 1, nil, 1)

	go func() {
		defer close(stoppedC)
		httpSrv.Stop()
		log.Println("Stopped http Server")
		elector.Stop()
		log.Println("Stopped Consul leadership elector.")
		sw.Stop()
		log.Println("Stopped Service Watcher.")
		nw.Stop()
		log.Println("Stopped Node Watcher.")

	}()

	stats.WithTiming("shutdown_time", nil, func() {
		// make sure stops don't take too long
		timer := time.NewTimer(stopTimeout)
		select {
		case <-timer.C:
			log.Println("goroutines took too long to stop. Exiting.")
		case <-stoppedC:
			log.Println("Stopped.")
		}
		os.Stdout.Sync()
	})
}
