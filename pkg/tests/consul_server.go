package tests

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"syscall"
	"time"

	capi "github.com/hashicorp/consul/api"
)

const ConsulPort = "28500" // some port that's not Consul's production port

type TestingConsulServer struct {
	cmd      *exec.Cmd
	NodeName string
	Client   *capi.Client
	Config   *capi.Config
}

func NewTestingConsulServer() *TestingConsulServer {
	config := capi.DefaultConfig()
	config.Address = fmt.Sprintf("127.0.0.1:%s", ConsulPort)
	nodeName := fmt.Sprintf("consul-test-server-%s", ConsulPort)
	cmd := exec.Command("consul", "agent", "-dev",
		"-http-port", ConsulPort, "-bind=127.0.0.1",
		"-node", nodeName)
	cmd.Stdout = ioutil.Discard
	cmd.Stderr = ioutil.Discard

	return &TestingConsulServer{
		cmd:      cmd,
		NodeName: nodeName,
		Config:   config}
}

// Start consul in dev mode
// Logs will go to stdout/stderr
// Each outer Test* func will get a freshly restarted consul
func (server *TestingConsulServer) Start() error {
	err := server.cmd.Start()
	if err != nil {
		return err
	}

	client, err := capi.NewClient(server.Config)
	if err != nil {
		return err
	}
	server.Client = client

	startedC := make(chan struct{})
	go func() {
		for {
			_, err := client.KV().Put(&capi.KVPair{Key: "test", Value: []byte("bar")}, nil)
			if err == nil {
				close(startedC)
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// make sure the start doesn't take to long
	timer := time.NewTimer(5 * time.Second)
	select {
	case <-timer.C:
		return errors.New("took too long to start consul")
	case <-startedC:
	}

	return nil
}

// Stop consul.  Wait up to 2 seconds before killing it forcefully
func (server *TestingConsulServer) Stop() error {
	server.cmd.Process.Signal(syscall.SIGTERM)
	stoppedC := make(chan struct{})
	go func() {
		defer close(stoppedC)
		server.cmd.Wait()
	}()

	// make sure the stop doesn't take to long
	timer := time.NewTimer(5 * time.Second)
	select {
	case <-timer.C:
		return errors.New("took too long to stop consul")
	case <-stoppedC:
		server.cmd.Process.Kill()
	}

	return nil
}
