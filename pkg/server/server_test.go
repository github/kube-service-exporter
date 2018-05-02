package server

import (
	"encoding/json"
	"io/ioutil"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func WithTestServer(t *testing.T, f func(*httptest.Server)) {
	t.Helper()
	// we just need a server to get the mux
	s := New("", 0, time.Duration(0))
	ts := httptest.NewServer(s.mux)
	defer ts.Close()

	f(ts)
}

func TestHealthz(t *testing.T) {
	WithTestServer(t, func(ts *httptest.Server) {
		client := ts.Client()
		res, err := client.Get(ts.URL + "/healthz")
		require.NoError(t, err)

		body, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		require.NoError(t, err)
		assert.Equal(t, "ok", string(body))
	})
}

func TestExpVar(t *testing.T) {
	WithTestServer(t, func(ts *httptest.Server) {
		client := ts.Client()
		res, err := client.Get(ts.URL + "/debug/vars")
		require.NoError(t, err)

		body, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		require.NoError(t, err)

		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		require.NoError(t, err)
		assert.Contains(t, data, "memstats")
	})
}
