package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfigure(t *testing.T) {
	var err error

	assert.Nil(t, Client(), "Client() should be nil before Configure()")

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
