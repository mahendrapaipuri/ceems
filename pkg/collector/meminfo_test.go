//go:build !nomemory
// +build !nomemory

package collector

import (
	"os"
	"testing"
)

func TestMemInfo(t *testing.T) {
	file, err := os.Open("testdata/proc/meminfo")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	memInfo, err := parseMemInfo(file)
	if err != nil {
		t.Fatal(err)
	}

	if want, got := 16042172416.0, memInfo["MemTotal_bytes"]; want != got {
		t.Errorf("want memory total %f, got %f", want, got)
	}

	if want, got := 16424894464.0, memInfo["DirectMap2M_bytes"]; want != got {
		t.Errorf("want memory directMap2M %f, got %f", want, got)
	}
}
