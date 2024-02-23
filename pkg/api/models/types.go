package models

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
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
