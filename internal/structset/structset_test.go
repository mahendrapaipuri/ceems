package structset

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// testStruct is a test struct that will be used in tests.
type testStruct struct {
	ID     int      `json:"-"                sql:"id"`
	Field1 string   `json:"field1,omitempty" sql:"f1"`
	Field2 bool     `json:"field2"           sql:"f2"`
	Field3 any      `sql:"f3"`
	Field4 []string `json:"field4"           sql:"f4"`
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

func TestCachedFiledIndexes(t *testing.T) {
	var value testStruct

	indexes := CachedFieldIndexes(reflect.TypeOf(&value).Elem())

	expected := map[string]int{"f1": 1, "f2": 2, "f3": 3, "f4": 4, "id": 0}
	assert.Equal(t, expected, indexes)

	// Get length of sync map
	var i int

	fieldIndexesCache.Range(func(k, v any) bool {
		i++

		return true
	})

	assert.Equal(t, 1, i)

	// Now making second request should get value from cache
	indexes = CachedFieldIndexes(reflect.TypeOf(&value).Elem())
	assert.Equal(t, expected, indexes)
}
