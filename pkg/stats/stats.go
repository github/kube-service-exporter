// stats is a very lightweight wrapper around the dogstatsd client, which gives
// us a package-level function
package stats

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/pkg/errors"
)

var client *statsd.Client

func Configure(host string, port int) error {
	var err error

	client, err = statsd.NewBuffered(fmt.Sprintf("%s:%d", host, port), 100)
	if err != nil {
		return errors.Wrap(err, "Error configuring statsd client")
	}
	client.Namespace = "kube-service-exporter."

	return nil
}

func Client() *statsd.Client {
	return client
}

func WithTiming(name string, tags []string, f func()) time.Duration {
	start := time.Now()
	f()
	took := time.Since(start)
	client.Timing(name, took, tags, 1)
	return took
}

func IncrSuccessOrFail(err error, prefix string, tags []string) {
	var key string

	if err == nil {
		key = prefix + ".success"
	} else {
		key = prefix + ".fail"
	}

	client.Incr(key, tags, 1)
}
