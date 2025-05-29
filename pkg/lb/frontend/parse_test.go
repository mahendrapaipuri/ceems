//go:build cgo
// +build cgo

package frontend

import (
	"bytes"
	"net/http"
	"net/url"
	"strings"
	"testing"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestParseTSDBQueryParams(t *testing.T) {
	tests := []struct {
		path   string
		query  string
		uuids  []string
		rmID   string
		rmIDs  []string
		method string
	}{
		{
			path:   "/api/v1/query",
			query:  "foo{uuid=~\"123|456\",gpuuuid=\"GPU-0123\",ceems_id=\"rm-0\"}",
			uuids:  []string{"123", "456"},
			rmID:   "rm-0",
			rmIDs:  []string{"rm-0", "rm-1"},
			method: "GET",
		},
		{
			path:   "/api/v1/query_range",
			query:  "foo{uuid=~\"abc-123|456\",ceems_id=\"rm-0|rm-1\"}",
			uuids:  []string{"abc-123", "456"},
			rmID:   "rm-1",
			rmIDs:  []string{"rm-0", "rm-1"},
			method: "POST",
		},
		{
			path:   "/api/v1/query_range",
			query:  "foo{uuid=\"456\",gpuuuid=\"GPU-0123\",ceems_id=\"rm-0\"}",
			uuids:  []string{"456"},
			rmID:   "rm-0",
			rmIDs:  []string{"rm-0"},
			method: "POST",
		},
		{
			path:   "/api/v1/series",
			query:  "foo{uuid=\"456\",gpuuuid=\"GPU-0123\",ceems_id=\"rm-0\"}",
			uuids:  []string{"456"},
			rmID:   "rm-0",
			rmIDs:  []string{"rm-0"},
			method: "GET",
		},
		{
			path:   "/api/v1/query_range",
			query:  "foo{uuid=~\"abc_123|456\"}",
			method: "POST",
		},
	}

	for _, test := range tests {
		var body *strings.Reader

		// Query params
		data := url.Values{}
		if strings.HasSuffix(test.path, "series") {
			data.Set("match[]", test.query)
		} else {
			data.Set("query", test.query)
		}

		if test.method == "POST" {
			body = strings.NewReader(data.Encode())
		} else {
			body = strings.NewReader("hello")
		}

		req, err := http.NewRequest(test.method, "http://localhost:9090"+test.path, body) //nolint:noctx
		require.NoError(t, err)

		// For GET request add query to URL
		if test.method == "GET" {
			req.URL.RawQuery = data.Encode()
		} else {
			req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		}

		p := &ReqParams{}
		err = parseTSDBRequest(p, req)
		require.NoError(t, err)

		assert.Equal(t, test.uuids, p.uuids)
		assert.Equal(t, test.rmID, p.clusterID)

		// Set parameters to request's context
		newReq := setQueryParams(req, p)

		if test.method == "POST" {
			// Check the new request body can still be parsed
			require.NoError(t, newReq.ParseForm())

			// Check if form value can be retrieved
			require.NotEmpty(t, newReq.FormValue("query"))
		}
	}
}

func TestParsePyroQueryParams(t *testing.T) {
	tests := []struct {
		resource string
		message  any
		uuids    []string
		start    int64
		rmIDs    string
	}{
		{
			resource: "SelectMergeStacktraces",
			message: &querierv1.SelectMergeStacktracesRequest{
				LabelSelector: `{service_name="123"}`,
				Start:         1735209190,
			},
			uuids: []string{"123"},
			start: 1735209000,
		},
		{
			resource: "SelectMergeStacktraces",
			message: &querierv1.SelectMergeStacktracesRequest{
				LabelSelector: `{service_name="123", ceems_id="default"}`,
				Start:         1735209190,
			},
			uuids: []string{"123"},
			rmIDs: "default",
			start: 1735209000,
		},
		{
			resource: "LabelNames",
			message: &typesv1.LabelNamesRequest{
				Matchers: []string{`{__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds", service_name="123", ceems_id="default"}`},
				Start:    1735209000,
			},
			uuids: []string{"123"},
			rmIDs: "default",
			start: 1735209000,
		},
		{
			resource: "LabelValues",
			message: &typesv1.LabelValuesRequest{
				Matchers: []string{`{__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds", service_name="123", ceems_id="default"}`},
				Start:    1735209000,
			},
			uuids: []string{"123"},
			rmIDs: "default",
			start: 1735209000,
		},
	}

	for _, test := range tests {
		var data []byte

		var err error
		// Query params
		switch test.resource {
		case "SelectMergeStacktraces":
			data, err = proto.Marshal(test.message.(*querierv1.SelectMergeStacktracesRequest)) //nolint:forcetypeassert
		case "LabelNames":
			data, err = proto.Marshal(test.message.(*typesv1.LabelNamesRequest)) //nolint:forcetypeassert
		case "LabelValues":
			data, err = proto.Marshal(test.message.(*typesv1.LabelValuesRequest)) //nolint:forcetypeassert
		}

		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, "http://localhost:4040/"+test.resource, bytes.NewBuffer(data)) //nolint:noctx
		require.NoError(t, err)

		p := &ReqParams{}
		err = parsePyroRequest(p, req)
		require.NoError(t, err)

		assert.Equal(t, test.uuids, p.uuids, test.resource)
		assert.Equal(t, test.rmIDs, p.clusterID, test.resource)
	}
}
