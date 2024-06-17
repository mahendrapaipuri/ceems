package frontend

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
)

func TestParseTimeParam(t *testing.T) {
	type resultType struct {
		asTime  time.Time
		asError func() error
	}

	ts, err := parseTime("1582468023986")
	if err != nil {
		t.Fatal("failed to parse time")
	}

	tests := []struct {
		paramName    string
		paramValue   string
		defaultValue time.Time
		result       resultType
	}{
		{ // When data is valid.
			paramName:    "start",
			paramValue:   "1582468023986",
			defaultValue: MinTime,
			result: resultType{
				asTime:  ts,
				asError: nil,
			},
		},
		{ // When data is empty string.
			paramName:    "end",
			paramValue:   "",
			defaultValue: MaxTime,
			result: resultType{
				asTime:  MaxTime,
				asError: nil,
			},
		},
		{ // When data is not valid.
			paramName:    "foo",
			paramValue:   "baz",
			defaultValue: MaxTime,
			result: resultType{
				asTime: time.Time{},
				asError: func() error {
					_, err := parseTime("baz")
					return fmt.Errorf("invalid time value for '%s': %w", "foo", err)
				},
			},
		},
	}

	for _, test := range tests {
		req, err := http.NewRequest("GET", "localhost:42/foo?"+test.paramName+"="+test.paramValue, nil)
		if err != nil {
			t.Fatal("failed to create request")
		}

		result := test.result
		if asTime, err := parseTimeParam(req, test.paramName, test.defaultValue); err != nil {
			if err.Error() != result.asError().Error() {
				t.Errorf("expected %s, got %s", result.asError(), err)
			}
		} else {
			if result.asTime != asTime {
				t.Errorf("%s: expected %s, got %s", test.paramValue, result.asTime, asTime)
			}
		}
	}
}

func TestParseTime(t *testing.T) {
	ts, err := time.Parse(time.RFC3339Nano, "2015-06-03T13:21:58.555Z")
	if err != nil {
		panic(err)
	}

	tests := []struct {
		input  string
		fail   bool
		result time.Time
	}{
		{
			input: "",
			fail:  true,
		},
		{
			input: "abc",
			fail:  true,
		},
		{
			input: "30s",
			fail:  true,
		},
		{
			input:  "123",
			result: time.Unix(123, 0),
		},
		{
			input:  "123.123",
			result: time.Unix(123, 123000000),
		},
		{
			input:  "2015-06-03T13:21:58.555Z",
			result: ts,
		},
		{
			input:  "2015-06-03T14:21:58.555+01:00",
			result: ts,
		},
		{
			// Test float rounding.
			input:  "1543578564.705",
			result: time.Unix(1543578564, 705*1e6),
		},
		{
			input:  MinTime.Format(time.RFC3339Nano),
			result: MinTime,
		},
		{
			input:  MaxTime.Format(time.RFC3339Nano),
			result: MaxTime,
		},
	}

	for _, test := range tests {
		ts, err := parseTime(test.input)
		if !test.fail {
			if err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if !ts.Equal(test.result) {
				t.Errorf("%s: expected %s, got %s", test.input, test.result, ts)
			}
			continue
		}
		if err == nil {
			t.Errorf("expected error %s", test.input)
		}
	}
}

func TestParseQueryParams(t *testing.T) {
	tests := []struct {
		query  string
		uuids  []string
		rmID   string
		rmIDs  []string
		method string
	}{
		{
			query:  "foo{uuid=~\"123|456\",gpuuuid=\"GPU-0123\",ceems_id=\"rm-0\"}",
			uuids:  []string{"123", "456"},
			rmID:   "rm-0",
			rmIDs:  []string{"rm-0", "rm-1"},
			method: "GET",
		},
		{
			query:  "foo{uuid=~\"abc-123|456\",ceems_id=\"rm-0|rm-1\"}",
			uuids:  []string{"abc-123", "456"},
			rmID:   "rm-1",
			rmIDs:  []string{"rm-0", "rm-1"},
			method: "POST",
		},
		{
			query:  "foo{uuid=\"456\",gpuuuid=\"GPU-0123\",ceems_id=\"rm-0\"}",
			uuids:  []string{"456"},
			rmID:   "rm-0",
			rmIDs:  []string{"rm-0"},
			method: "POST",
		},
		{
			query:  "foo{uuid=~\"abc_123|456\"}",
			method: "POST",
		},
	}

	for _, test := range tests {
		var body *strings.Reader

		// Query params
		data := url.Values{}
		data.Set("query", test.query)
		if test.method == "POST" {
			body = strings.NewReader(data.Encode())
		} else {
			body = strings.NewReader("hello")
		}
		req, err := http.NewRequest(test.method, "http://localhost:9090", body)
		if err != nil {
			t.Fatal(err)
		}

		// For GET request add query to URL
		if test.method == "GET" {
			req.URL.RawQuery = data.Encode()
		} else {
			req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		}

		newReq := parseQueryParams(req, test.rmIDs, log.NewNopLogger())
		queryParams := newReq.Context().Value(QueryParamsContextKey{}).(*QueryParams)
		if !reflect.DeepEqual(queryParams.uuids, test.uuids) {
			t.Errorf("%s: expected %v, got %v", test.query, test.uuids, queryParams.uuids)
			continue
		}

		if queryParams.id != test.rmID {
			t.Errorf("%s: expected %s, got %s", test.query, test.rmID, queryParams.id)
			continue
		}

		if test.method == "POST" {
			// Check the new request body can still be parsed
			if err = newReq.ParseForm(); err != nil {
				t.Errorf("%s: cannot parse new request: %s", test.query, err)
			}

			// Check if form value can be retrieved
			if val := newReq.FormValue("query"); val == "" {
				t.Errorf("%s: expected query found none", test.query)
			}
		}
	}
}
