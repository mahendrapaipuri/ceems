package collector

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
)

const mockCEEMSExporterAppName = "mockApp"

var mockCEEMSExporterApp = *kingpin.New(
	mockCEEMSExporterAppName,
	"Prometheus Exporter to export compute metrics.",
)

func queryExporter(address string) error {
	resp, err := http.Get(fmt.Sprintf("http://%s/metrics", address))
	if err != nil {
		return err
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := resp.Body.Close(); err != nil {
		return err
	}
	if want, have := http.StatusOK, resp.StatusCode; want != have {
		return fmt.Errorf("want /metrics status code %d, have %d. Body:\n%s", want, have, b)
	}
	return nil
}

func TestCEEMSExporterAppHandler(t *testing.T) {
	a := CEEMSExporter{
		appName: mockCEEMSExporterAppName,
		App:     mockCEEMSExporterApp,
	}

	// Create handler
	handler := a.newHandler(false, 1, log.NewNopLogger())
	if handler.maxRequests != 1 {
		t.Errorf("Expected maxRequests to %d. Got %d", 1, handler.maxRequests)
	}
}

func TestCEEMSExporterMain(t *testing.T) {
	// Remove test related args and add a dummy arg
	os.Args = append([]string{os.Args[0]}, "--web.max-requests=2")
	a := CEEMSExporter{
		appName: mockCEEMSExporterAppName,
		App:     mockCEEMSExporterApp,
	}

	// Start Main
	go func() {
		a.Main()
	}()

	// Query exporter
	for i := 0; i < 10; i++ {
		if err := queryExporter("localhost:9010"); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
		if i == 9 {
			t.Errorf("Could not start exporter after %d attempts", i)
		}
	}
}
