//go:build cgo
// +build cgo

package frontend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"google.golang.org/protobuf/proto"
)

// Set query params into request's context and return new request.
func setQueryParams(r *http.Request, queryParams *ReqParams) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ReqParamsContextKey{}, queryParams))
}

// parseTSDBRequest parses TSDB query in the request after cloning it and reads them into request params.
func parseTSDBRequest(p *ReqParams, r *http.Request) error {
	var body []byte

	var err error

	// Make a new request and add newReader to that request body
	clonedReq := r.Clone(r.Context())

	// If request has no body go to proxy directly
	if r.Body == nil {
		return errors.New("no body found in the request")
	}

	// If failed to read body, skip verification and go to request proxy
	if body, err = io.ReadAll(r.Body); err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// clone body to existing request and new request
	r.Body = io.NopCloser(bytes.NewReader(body))
	clonedReq.Body = io.NopCloser(bytes.NewReader(body))

	// Get form values
	if err = clonedReq.ParseForm(); err != nil {
		return fmt.Errorf("failed to parse request form data: %w", err)
	}

	// Except for query API, rest of the load balanced API endpoint have start query param
	var targetQueryParam string

	switch {
	case strings.HasSuffix(clonedReq.URL.Path, "query"):
		targetQueryParam = "query"
	case strings.HasSuffix(clonedReq.URL.Path, "query_range"):
		targetQueryParam = "query"
	default:
		targetQueryParam = "match[]"
	}

	// Parse TSDB's query in request query params
	if vals, ok := clonedReq.Form[targetQueryParam]; ok {
		for _, val := range vals {
			parseReqParams(p, val)
		}
	}

	return nil
}

// parsePyroRequest parses Pyroscope query in the request after cloning it and reads them into request params.
func parsePyroRequest(p *ReqParams, r *http.Request) error {
	var body []byte

	var err error

	// If request has no body go to proxy directly
	if r.Body == nil {
		return errors.New("no body found in the request")
	}

	// If failed to read body, skip verification and go to request proxy
	if body, err = io.ReadAll(r.Body); err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// clone body to existing request
	r.Body = io.NopCloser(bytes.NewReader(body))

	// Read body into request data based on resource
	switch {
	case strings.HasSuffix(r.URL.Path, "SelectMergeStacktraces"):
		// Read body into request data
		data := querierv1.SelectMergeStacktracesRequest{}
		if err := proto.Unmarshal(body, &data); err != nil {
			return fmt.Errorf("failed to umarshall request body: %w", err)
		}

		// Parse Pyroscope's LabelSelector in request data
		if val := data.GetLabelSelector(); val != "" {
			parseReqParams(p, val)
		}
	case strings.HasSuffix(r.URL.Path, "LabelNames"):
		// Read body into request data
		data := typesv1.LabelNamesRequest{}
		if err := proto.Unmarshal(body, &data); err != nil {
			return fmt.Errorf("failed to umarshall request body: %w", err)
		}

		// Parse Pyroscope's LabelSelector in request data
		if vals := data.GetMatchers(); vals != nil {
			for _, val := range vals {
				parseReqParams(p, val)
			}
		}
	case strings.HasSuffix(r.URL.Path, "LabelValues"):
		// Read body into request data
		data := typesv1.LabelValuesRequest{}
		if err := proto.Unmarshal(body, &data); err != nil {
			return fmt.Errorf("failed to umarshall request body: %w", err)
		}

		// Parse Pyroscope's LabelSelector in request data
		if vals := data.GetMatchers(); vals != nil {
			for _, val := range vals {
				parseReqParams(p, val)
			}
		}
	}

	return nil
}

// parseRequestParams parses request parameters from `req` and reads them into `p`.
func parseReqParams(p *ReqParams, req string) {
	// Extract UUIDs from query
	for _, match := range regexpUUID.FindAllStringSubmatch(req, -1) {
		if len(match) > 1 {
			for uuid := range strings.SplitSeq(match[1], "|") {
				// Ignore empty strings
				if strings.TrimSpace(uuid) != "" && !slices.Contains(p.uuids, uuid) {
					p.uuids = append(p.uuids, uuid)
				}
			}
		}
	}

	// Extract ceems_id from query. If multiple values are provided, always
	// get the last and most recent one
	idMatches := regexID.FindAllStringSubmatch(req, -1)
	for _, match := range idMatches {
		if len(match) > 1 {
			for idMatch := range strings.SplitSeq(match[1], "|") {
				// Ignore empty strings
				if strings.TrimSpace(idMatch) != "" {
					p.clusterID = strings.TrimSpace(idMatch)
				}
			}
		}
	}
}
