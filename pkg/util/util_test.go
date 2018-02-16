package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringInSlice(t *testing.T) {
	strs := []string{"wicked", "cool", "super"}

	for _, str := range strs {
		assert.True(t, StringInSlice(str, strs))
	}
	assert.False(t, StringInSlice("Wicked", strs))
	assert.False(t, StringInSlice("rad", strs))
}
