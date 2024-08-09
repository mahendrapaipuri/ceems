package models

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/prometheus/common/config"
	"gopkg.in/yaml.v3"
)

// Infinity and NaN representations.
var (
	infNaNRepr = []string{
		"+Inf", "\"+Inf\"", "-Inf", "\"-Inf\"", "+inf", "\"+inf\"",
		"-inf", "\"-inf\"", "Inf", "\"Inf\"", "inf", "\"inf\"",
		"Infinity", "\"Infinity\"", "infinity", "\"infinity\"",
		"-Infinity", "\"-Infinity\"", "-infinity", "\"-infinity\"",
		"+Infinity", "\"+Infinity\"", "+infinity", "\"+infinity\"",
		"NaN", "\"NaN\"", "nan", "\"nan\"",
	}
)

// Generic is map to store any mixed data types. Only string and int are supported.
// Any number will be converted into int64.
// Ref: https://go.dev/play/p/89ra6QgcZba, https://husobee.github.io/golang/database/2015/06/12/scanner-valuer.html,
// https://gist.github.com/jmoiron/6979540
type Generic map[string]interface{}

// Value implements Valuer interface.
func (g Generic) Value() (driver.Value, error) {
	var generic []byte

	var err error
	if generic, err = json.Marshal(g); err != nil {
		return nil, err
	}

	return driver.Value(string(generic)), nil
}

// Scan implements Scanner interface.
func (g *Generic) Scan(v interface{}) error {
	if v == nil {
		return nil
	}

	// Initialise a json decoder
	var d *json.Decoder

	switch data := v.(type) {
	case string:
		d = json.NewDecoder(bytes.NewReader([]byte(data)))
	case []byte:
		d = json.NewDecoder(bytes.NewReader(data))
	default:
		return fmt.Errorf("cannot scan type %T! into Generic", v)
	}

	// Ref: Improvable, see https://groups.google.com/g/golang-nuts/c/TDuGDJAIuVM?pli=1
	// Decode into a tmp var
	var tmp map[string]interface{}

	d.UseNumber()

	if err := d.Decode(&tmp); err != nil {
		return err
	}

	// Convert json.Number to int64
	for k := range tmp {
		switch tmpt := tmp[k].(type) {
		case json.Number:
			if i, err := tmpt.Int64(); err == nil {
				tmp[k] = i
			}
		}
	}

	*g = tmp

	return nil
}

// Tag is a type alias to Generic that stores metadata of compute units.
type Tag = Generic

// Allocation is a type alias to Generic that stores allocation data of compute units.
type Allocation = Generic

// MetricMap is a type alias to Generic that stores arbritrary metrics as a map.
type MetricMap map[string]JSONFloat

// Value implements Valuer interface.
func (m MetricMap) Value() (driver.Value, error) {
	var generic []byte

	var err error
	if generic, err = json.Marshal(m); err != nil {
		return nil, err
	}

	return driver.Value(string(generic)), nil
}

// Scan implements Scanner interface.
func (m *MetricMap) Scan(v interface{}) error {
	if v == nil {
		return nil
	}

	// Initialise a json decoder
	var d *json.Decoder

	switch data := v.(type) {
	case string:
		d = json.NewDecoder(bytes.NewReader([]byte(data)))
	case []byte:
		d = json.NewDecoder(bytes.NewReader(data))
	default:
		return fmt.Errorf("cannot scan type %T! into MetricMap", v)
	}

	// Ref: Improvable, see https://groups.google.com/g/golang-nuts/c/TDuGDJAIuVM?pli=1
	// Decode into a tmp var
	var tmp map[string]JSONFloat
	if err := d.Decode(&tmp); err != nil {
		return err
	}

	*m = tmp

	return nil
}

// JSONFloat is a custom float64 that can handle Inf and NaN during JSON (un)marshalling.
type JSONFloat float64

// Value implements Valuer interface.
func (j JSONFloat) Value() (driver.Value, error) {
	var generic []byte

	var err error
	if generic, err = json.Marshal(j); err != nil {
		return nil, err
	}

	return driver.Value(string(generic)), nil
}

// Scan implements Scanner interface.
func (j *JSONFloat) Scan(v interface{}) error {
	if v == nil {
		return nil
	}

	// Initialise a json decoder
	var d *json.Decoder

	switch data := v.(type) {
	case string:
		if slices.Contains(infNaNRepr, data) {
			d = json.NewDecoder(bytes.NewReader([]byte("0")))
		} else {
			d = json.NewDecoder(bytes.NewReader([]byte(data)))
		}
	case []byte:
		d = json.NewDecoder(bytes.NewReader(data))
	case float64:
		if math.IsInf(data, 0) || math.IsNaN(data) {
			d = json.NewDecoder(bytes.NewReader([]byte("0")))
		} else {
			d = json.NewDecoder(bytes.NewReader([]byte(strconv.FormatFloat(data, 'E', -1, 64))))
		}
	default:
		return fmt.Errorf("cannot scan type %T! into JSONFloat", v)
	}

	// Ref: Improvable, see https://groups.google.com/g/golang-nuts/c/TDuGDJAIuVM?pli=1
	// Decode into a tmp var
	var tmp JSONFloat
	if err := d.Decode(&tmp); err != nil {
		return err
	}

	*j = tmp

	return nil
}

// MarshalJSON marshals JSONFloat into byte array
// The custom marshal interface will truncate the float64 to 8 decimals as storing
// all decimals will bring a very low added value and high DB storage.
func (j JSONFloat) MarshalJSON() ([]byte, error) {
	v := float64(j)
	if math.IsInf(v, 0) || math.IsNaN(v) {
		// handle infinity, assign desired value to v
		s := "0"

		return []byte(s), nil
	}

	// If v is actually a int, use json.Marshal else truncate the decimals to 8
	if v == float64(int(v)) {
		return json.Marshal(v)
	} else {
		// Convert to bytes by truncating to 8 decimals
		return []byte(fmt.Sprintf("%.8f", v)), nil
	}
}

// UnmarshalJSON unmarshals byte array into JSONFloat.
func (j *JSONFloat) UnmarshalJSON(v []byte) error {
	if s := string(v); slices.Contains(infNaNRepr, s) {
		*j = JSONFloat(0)

		return nil
	}
	// just a regular float value
	var fv float64
	if err := json.Unmarshal(v, &fv); err != nil {
		return err
	}

	*j = JSONFloat(fv)

	return nil
}

// List is a generic type to store slices. Only string and int slices are supported.
// Any number will be converted into int64.
type List []interface{}

// Value implements Valuer interface.
func (l List) Value() (driver.Value, error) {
	var list []byte

	var err error
	if list, err = json.Marshal(l); err != nil {
		return nil, err
	}

	return driver.Value(string(list)), nil
}

// Scan implements Scanner interface.
func (l *List) Scan(v interface{}) error {
	if v == nil {
		return nil
	}

	// Initialise a json decoder
	var d *json.Decoder

	switch data := v.(type) {
	case string:
		d = json.NewDecoder(bytes.NewReader([]byte(data)))
	case []byte:
		d = json.NewDecoder(bytes.NewReader(data))
	default:
		return fmt.Errorf("cannot scan type %T! into List", v)
	}

	// Ref: Improvable, see https://groups.google.com/g/golang-nuts/c/TDuGDJAIuVM?pli=1
	// Decode into a tmp var
	var tmp []interface{}

	d.UseNumber()

	if err := d.Decode(&tmp); err != nil {
		return err
	}

	// Convert json.Number to int64
	for k := range tmp {
		switch tmpt := tmp[k].(type) {
		case json.Number:
			if i, err := tmpt.Int64(); err == nil {
				tmp[k] = i
			}
		}
	}

	*l = tmp

	return nil
}

// WebConfig contains the client related configuration of a REST API server.
type WebConfig struct {
	URL              string                  `yaml:"url"`
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
}

// SetDirectory joins any relative file paths with dir.
func (c *WebConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *WebConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain WebConfig

	*c = WebConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	return c.HTTPClientConfig.Validate()
}

// CLIConfig contains the configuration of CLI client.
type CLIConfig struct {
	Path    string            `yaml:"path"`
	EnvVars map[string]string `yaml:"environment_variables"`
}

// Cluster contains the configuration of the given resource manager.
type Cluster struct {
	ID       string    `json:"id"      sql:"cluster_id"       yaml:"id"`
	Manager  string    `json:"manager" sql:"resource_manager" yaml:"manager"`
	Web      WebConfig `json:"-"       yaml:"web"`
	CLI      CLIConfig `json:"-"       yaml:"cli"`
	Updaters []string  `json:"-"       yaml:"updaters"`
	Extra    yaml.Node `json:"-"       yaml:"extra_config"`
}

// ClusterUnits is the container for the units and config of a given cluster.
type ClusterUnits struct {
	Cluster Cluster
	Units   []Unit
}

// ClusterProjects is the container for the projects for a given cluster.
type ClusterProjects struct {
	Cluster  Cluster
	Projects []Project
}

// ClusterUsers is the container for the users for a given cluster.
type ClusterUsers struct {
	Cluster Cluster
	Users   []User
}
