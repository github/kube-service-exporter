package tests

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	capi "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/require"
)

type TestingConsulServer struct {
	cmd      *exec.Cmd
	t        *testing.T
	NodeName string
	Client   *capi.Client
	Config   *capi.Config
}

func NewTestingConsulServer(t *testing.T) *TestingConsulServer {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()
	config := capi.DefaultConfig()
	config.Address = listener.Addr().String()

	// find a random unused port for Consul to listen on just to reduce the
	// probability that we talk to a production Consul.  This is racey, but
	// should be fine since it's unlikely someone is going to run a consul on
	// our random port between closing this dummy listener and starting aConsul.
	nodeName := fmt.Sprintf("consul-test-server-%s", port)
	cmd := exec.Command("consul", "agent", "-dev",
		"-http-port", port, "-bind=127.0.0.1",
		"-node", nodeName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return &TestingConsulServer{
		cmd:      cmd,
		t:        t,
		NodeName: nodeName,
		Config:   config}
}

// Start consul in dev mode on a random port for testing against
// Logs will go to stdout/stderr
// Each outer Test* func will get a freshly restarted consul
func (server *TestingConsulServer) Start() {
	server.t.Helper()
	err := server.cmd.Start()
	require.NoError(server.t, err)

	client, err := capi.NewClient(server.Config)
	require.NoError(server.t, err)
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
	timer := time.NewTimer(2 * time.Second)
	select {
	case <-timer.C:
		server.t.Fatal("Took too long to start consul")
	case <-startedC:
	}
}

// Stop consul.  Wait up to 2 seconds before killing it forcefully
func (server *TestingConsulServer) Stop() {
	server.t.Helper()
	server.cmd.Process.Signal(syscall.SIGINT)
	stoppedC := make(chan struct{})
	go func() {
		defer close(stoppedC)
		server.cmd.Wait()
	}()

	// make sure the stop doesn't take to long
	timer := time.NewTimer(2 * time.Second)
	select {
	case <-timer.C:
		server.t.Fatal("Took too long to stop consul")
	case <-stoppedC:
		server.cmd.Process.Kill()
	}
}
