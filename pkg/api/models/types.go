package models

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"

	"github.com/prometheus/common/config"
	"gopkg.in/yaml.v3"
)

// Generic is map to store any mixed data types. Only string and int are supported.
// Any number will be converted into int64.
// Ref: https://go.dev/play/p/89ra6QgcZba, https://husobee.github.io/golang/database/2015/06/12/scanner-valuer.html,
// https://gist.github.com/jmoiron/6979540
type Generic map[string]interface{}

// Value implements Valuer interface
func (g Generic) Value() (driver.Value, error) {
	var generic []byte
	var err error
	if generic, err = json.Marshal(g); err != nil {
		return nil, err
	}
	return driver.Value(string(generic)), nil
}

// Scan implements Scanner interface
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
		return fmt.Errorf("cannot scan type %t into Map", v)
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

// Tag is a type alias to Generic that stores metadata of compute units
type Tag = Generic

// Allocation is a type alias to Generic that stores allocation data of compute units
type Allocation = Generic

// JSONFloat is a custom float64 that can handle Inf and NaN during JSON (un)marshalling
type JSONFloat float64

// MarshalJSON marshals JSONFloat into byte array
func (j JSONFloat) MarshalJSON() ([]byte, error) {
	v := float64(j)
	if math.IsInf(v, 0) || math.IsNaN(v) {
		// handle infinity, assign desired value to v
		s := "0"
		return []byte(s), nil
	}
	return json.Marshal(v) // marshal result as standard float64
}

// UnmarshalJSON unmarshals byte array into JSONFloat
func (j *JSONFloat) UnmarshalJSON(v []byte) error {
	if s := string(v); s == "+Inf" || s == "-Inf" || s == "NaN" {
		// if +Inf/-Inf indiciates infinity
		if s == "+Inf" {
			*j = JSONFloat(math.Inf(1))
			return nil
		} else if s == "-Inf" {
			*j = JSONFloat(math.Inf(-1))
			return nil
		}
		*j = JSONFloat(math.NaN())
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

// WebConfig contains the client related configuration of a REST API server
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

// CLIConfig contains the configuration of CLI client
type CLIConfig struct {
	Path    string            `yaml:"path"`
	EnvVars map[string]string `yaml:"environment_variables"`
}

// Cluster contains the configuration of the given resource manager
type Cluster struct {
	ID       string    `yaml:"id"           json:"id"      sql:"cluster_id"`
	Manager  string    `yaml:"manager"      json:"manager" sql:"resource_manager"`
	Web      WebConfig `yaml:"web"          json:"-"`
	CLI      CLIConfig `yaml:"cli"          json:"-"`
	Updaters []string  `yaml:"updaters"     json:"-"`
	Extra    yaml.Node `yaml:"extra_config" json:"-"`
}

// ClusterUnits is the container for the units and config of a given cluster
type ClusterUnits struct {
	Cluster Cluster
	Units   []Unit
}
