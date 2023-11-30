//go:build !utils
// +build !utils

package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/zeebo/xxh3"
)

const (
	OPENDATASOFT_API_PATH    = `%s/api/records/1.0/search/?%s`
	OPENDATASOFT_API_BASEURL = `https://odre.opendatasoft.com`
)

var (
	nodelistRegExp = regexp.MustCompile(`(\[\d+\-\d+\])`)
)

// Execute command and return stdout/stderr
func Execute(cmd string, args []string, logger log.Logger) ([]byte, error) {
	level.Debug(logger).Log("msg", "Executing", "command", cmd, "args", fmt.Sprintf("%+v", args))
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		err = fmt.Errorf("error running %s: %s", cmd, err)
	}
	return out, err
}

// Get all fields in a given struct
func GetStructFieldName(Struct interface{}) []string {
	var fields []string

	v := reflect.ValueOf(Struct)
	typeOfS := v.Type()

	for i := 0; i < v.NumField(); i++ {
		fields = append(fields, typeOfS.Field(i).Name)
	}
	return fields
}

// Get all values in a given struct
func GetStructFieldValue(Struct interface{}) []interface{} {
	v := reflect.ValueOf(Struct)
	values := make([]interface{}, v.NumField())

	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		values = append(values, f.Interface())
	}
	return values
}

// Expand SLURM NODELIST into slice of nodenames
func NodelistParser(nodelistExp string) []string {
	var nodeNames []string
	// First split by , to get individual nodes
	for _, nodeexp := range strings.Split(nodelistExp, ",") {
		// If it contains "[", it means they are range of nodes
		if strings.Contains(nodeexp, "[") {
			matches := nodelistRegExp.FindAllString(nodeexp, -1)
			if len(matches) == 0 {
				continue
			}
			// Get only first match as we use recursion
			for _, match := range matches[0:1] {
				matchSansBrackets := match[1 : len(match)-1]
				startIdx, err := strconv.Atoi(strings.Split(matchSansBrackets, "-")[0])
				if err != nil {
					continue
				}
				endIdx, err := strconv.Atoi(strings.Split(matchSansBrackets, "-")[1])
				if err != nil {
					continue
				}
				for i := startIdx; i <= endIdx; i++ {
					nodename := strings.Replace(nodeexp, match, strconv.Itoa(i), -1)
					// Add them to slice and call function again
					nodeNames = append(nodeNames, NodelistParser(nodename)...)
				}
			}

		} else {
			nodeNames = append(nodeNames, regexp.QuoteMeta(nodeexp))
		}
	}
	return nodeNames
}

// Load cgroups v2 metrics from a given path
func LoadCgroupsV2Metrics(
	name string,
	cgroupfsPath string,
	controllers []string,
) (map[string]float64, error) {
	data := make(map[string]float64)

	for _, fName := range controllers {
		contents, err := os.ReadFile(filepath.Join(cgroupfsPath, name, fName))
		if err != nil {
			return data, err
		}
		for _, line := range strings.Split(string(contents), "\n") {
			// Some of the above have a single value and others have a "data_name 123"
			parts := strings.Fields(line)
			indName := fName
			indData := 0
			if len(parts) == 1 || len(parts) == 2 {
				if len(parts) == 2 {
					indName += "." + parts[0]
					indData = 1
				}
				if parts[indData] == "max" {
					data[indName] = -1.0
				} else {
					f, err := strconv.ParseFloat(parts[indData], 64)
					if err == nil {
						data[indName] = f
					} else {
						return data, err
					}
				}
			}
		}
	}
	return data, nil
}

// Request to OPENDATASOFT API to get RTE energy data for France
func GetRteEnergyMixData() (float64, error) {
	params := url.Values{}
	params.Add("dataset", "eco2mix-national-tr")
	params.Add("facet", "nature")
	params.Add("facet", "date_heure")
	params.Add("start", "0")
	params.Add("rows", "1")
	params.Add("sort", "date_heure")
	params.Add(
		"q",
		fmt.Sprintf(
			"date_heure:[%s TO #now()] AND NOT #null(taux_co2)",
			time.Now().Format("2006-01-02"),
		),
	)
	queryString := params.Encode()

	resp, err := http.DefaultClient.Get(
		fmt.Sprintf(OPENDATASOFT_API_PATH, OPENDATASOFT_API_BASEURL, queryString),
	)
	if err != nil {
		return -1, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return -1, err
	}

	var data nationalRealTimeResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return -1, err
	}

	var fields []nationalRealTimeFields
	for _, r := range data.Records {
		fields = append(fields, r.Fields)
	}
	return float64(fields[0].TauxCo2), nil
}

// Get a UUID5 for given slice of strings
func GetUuidFromString(stringSlice []string) (string, error) {
	s := strings.Join(stringSlice[:], ",")
	h := xxh3.HashString128(s).Bytes()
	uuid, err := uuid.FromBytes(h[:])
	// hash := md5.Sum([]byte(s))
	// md5string := hex.EncodeToString(hash[:])
	// // generate the UUID from the
	// // first 16 bytes of the MD5 hash
	// uuid, err := uuid.FromBytes([]byte(md5string[0:16]))
	return uuid.String(), err
}
