package http

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/stretchr/testify/require"
)

func getCORSHandlerFunc() http.Handler {
	reg := regexp.MustCompile(`^https://foo\.com$`)
	cors := newCORS(reg, []string{base.GrafanaUserHeader})

	hf := func(w http.ResponseWriter, r *http.Request) {
		cors.set(w, r)
		w.WriteHeader(http.StatusOK)
	}

	return http.HandlerFunc(hf)
}

func TestCORSHandler(t *testing.T) {
	serverMux := http.NewServeMux()

	server := httptest.NewServer(serverMux)
	defer server.Close()

	client := &http.Client{}

	ch := getCORSHandlerFunc()
	serverMux.Handle("/any_path", ch)

	dummyOrigin := "https://foo.com"

	// OPTIONS with legit origin
	req, err := http.NewRequest(http.MethodOptions, server.URL+"/any_path", nil) //nolint:noctx
	require.NoError(t, err, "could not create request")

	req.Header.Set("Origin", dummyOrigin)

	resp, err := client.Do(req)
	require.NoError(t, err, "client get failed with unexpected error")
	defer resp.Body.Close()

	AccessControlAllowOrigin := resp.Header.Get("Access-Control-Allow-Origin")

	require.Equal(t, dummyOrigin, AccessControlAllowOrigin, "expected Access-Control-Allow-Origin header")

	// OPTIONS with bad origin
	req, err = http.NewRequest(http.MethodOptions, server.URL+"/any_path", nil) //nolint:noctx
	require.NoError(t, err, "could not create request")

	req.Header.Set("Origin", "https://not-foo.com")

	resp, err = client.Do(req)
	require.NoError(t, err, "client get failed with unexpected error")
	defer resp.Body.Close()

	AccessControlAllowOrigin = resp.Header.Get("Access-Control-Allow-Origin")
	require.Empty(t, AccessControlAllowOrigin, "Access-Control-Allow-Origin header should not exist but it was set")
}
