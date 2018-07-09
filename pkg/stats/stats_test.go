package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfigure(t *testing.T) {
	var err error

	assert.Nil(t, Client(), "Client() should be nil before Configure()")

	err = Client().Gauge("foo", 1, nil, 1)
	assert.NoError(t, err, "Sending a metric to a nil client should not throw an error")
	dur := WithTiming("test", nil, func() {
		time.Sleep(10 * time.Millisecond)
	})
	assert.True(t, dur >= 10*time.Millisecond, "WithTiming should work with an unconfigured client")

	err = Configure("127.0.0.1", 8125)
	assert.Nil(t, err, "Configure should not return an error")
	assert.NotNil(t, Client(), "Client should not be nil after configure")
}

func TestWithTiming(t *testing.T) {
	err := Configure("127.0.0.1", 8125)
	assert.Nil(t, err, "Configure should not return an error")

	dur := WithTiming("test", nil, func() {
		time.Sleep(10 * time.Millisecond)
	})
	assert.True(t, dur >= 10*time.Millisecond)
}
