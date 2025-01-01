package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"google.golang.org/protobuf/proto"
)

func main() {
	url := flag.String(
		"url",
		"http://localhost:4040/querier.v1.QuerierService/SelectMergeStacktraces",
		"URL to make HTTP request",
	)
	userName := flag.String(
		"username",
		"",
		"Username",
	)
	clusterID := flag.String(
		"cluster-id",
		"",
		"Cluster ID",
	)
	serviceName := flag.String(
		"uuid",
		"",
		"UUID",
	)
	startTime := flag.Int64(
		"start",
		time.Now().Unix(),
		"Unix time stamp",
	)

	flag.Parse()

	// Query params
	message := &querierv1.SelectMergeStacktracesRequest{
		LabelSelector: fmt.Sprintf(`{service_name="%s"}`, *serviceName),
		Start:         *startTime,
	}

	data, err := proto.Marshal(message)
	if err != nil {
		log.Fatalln("failed to marshal message", err)
	}

	req, err := http.NewRequest(http.MethodPost, *url, bytes.NewBuffer(data)) //nolint:noctx
	if err != nil {
		log.Fatalln("failed to create new request", err)
	}

	// Add necessary headers
	req.Header.Add("X-Grafana-User", *userName)
	req.Header.Add("X-Ceems-Cluster-Id", *clusterID)

	// Make request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalln("failed to make request", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln("failed to read response body", err)
	}

	if resp.StatusCode == http.StatusOK {
		// Unpack into data
		respData := &querierv1.SelectMergeStacktracesResponse{}
		if err = proto.Unmarshal(body, respData); err != nil {
			log.Fatalln("failed to umarshal proto response body", err)
		}

		fmt.Println("\nreceived profiles for ", strings.Join(respData.GetFlamegraph().GetNames(), ",")) //nolint:forbidigo
	} else {
		fmt.Println(string(body)) //nolint:forbidigo
	}
}
