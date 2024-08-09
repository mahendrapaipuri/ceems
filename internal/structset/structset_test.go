package structset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// testStruct is a test struct that will be used in tests.
type testStruct struct {
	ID     int         `json:"-"                sql:"id"`
	Field1 string      `json:"field1,omitempty" sql:"f1"`
	Field2 bool        `json:"field2"           sql:"f2"`
	Field3 interface{} `sql:"f3"`
	Field4 []string    `json:"field4"           sql:"f4"`
}

func TestStructFieldNames(t *testing.T) {
	fields := StructFieldNames(testStruct{})
	expectedFields := []string{"ID", "Field1", "Field2", "Field3", "Field4"}
	assert.ElementsMatch(t, fields, expectedFields)
}

func TestStructFieldValues(t *testing.T) {
	tags := StructFieldTagValues(testStruct{}, "json")
	expectedTags := []string{"field1", "field2", "Field3", "field4"}
	assert.ElementsMatch(t, tags, expectedTags)
}

func TestGetStructFieldTagMap(t *testing.T) {
	tagMap := StructFieldTagMap(testStruct{}, "json", "sql")
	expectedTagMap := map[string]string{
		"":       "id",
		"field1": "f1",
		"field2": "f2",
		"Field3": "f3",
		"field4": "f4",
	}
	assert.Equal(t, expectedTagMap, tagMap)
}
