package structset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// testStruct is a test struct that will be used in tests
type testStruct struct {
	ID     int         `json:"id"     sql:"id"`
	Field1 string      `json:"field1" sql:"f1"`
	Field2 bool        `json:"field2" sql:"f2"`
	Field3 interface{} `json:"field3" sql:"f3"`
	Field4 []string    `json:"field4" sql:"f4"`
}

func TestGetStructFieldNames(t *testing.T) {
	fields := GetStructFieldNames(testStruct{})
	expectedFields := []string{"ID", "Field1", "Field2", "Field3", "Field4"}
	assert.ElementsMatch(t, fields, expectedFields)
}

func TestGetStructFieldValues(t *testing.T) {
	tags := GetStructFieldTagValues(testStruct{}, "json")
	expectedTags := []string{"field1", "field2", "field3", "field4"}
	assert.ElementsMatch(t, tags, expectedTags)
}

func TestGetStructFieldTagMap(t *testing.T) {
	tagMap := GetStructFieldTagMap(testStruct{}, "json", "sql")
	expectedTagMap := map[string]string{
		"id":     "id",
		"field1": "f1",
		"field2": "f2",
		"field3": "f3",
		"field4": "f4",
	}
	assert.Equal(t, tagMap, expectedTagMap)
}
