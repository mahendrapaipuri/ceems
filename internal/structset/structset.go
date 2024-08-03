// Package structset implements helper functions that involves structs
package structset

import (
	"database/sql"
	"reflect"
	"strings"
	"sync"
)

var (
	fieldIndexesCache sync.Map
)

// GetStructFieldNames returns all fields in a given struct
func GetStructFieldNames(Struct interface{}) []string {
	var fields []string

	v := reflect.ValueOf(Struct)
	typeOfS := v.Type()

	for i := 0; i < v.NumField(); i++ {
		fields = append(fields, typeOfS.Field(i).Name)
	}
	return fields
}

// // GetStructFieldValues returns all values in a given struct
// func GetStructFieldValues(Struct interface{}) []interface{} {
// 	v := reflect.ValueOf(Struct)
// 	values := make([]interface{}, v.NumField())

// 	for i := 0; i < v.NumField(); i++ {
// 		f := v.Field(i)
// 		values = append(values, f.Interface())
// 	}
// 	return values
// }

// Get tag value of field. If tag value is "-", empty string will be returned
// If tag is empty, return name of field
func getTagValue(field reflect.StructField, tag string) string {
	if field.Tag.Get(tag) == "-" {
		return ""
	} else if field.Tag.Get(tag) == "" {
		return field.Name
	} else {
		return strings.Split(field.Tag.Get(tag), ",")[0]
	}
}

// GetStructFieldTagValues returns all tag names in a given struct for a given tag
func GetStructFieldTagValues(Struct interface{}, tag string) []string {
	v := reflect.ValueOf(Struct)
	typeOfS := v.Type()

	var values []string
	for i := 0; i < v.NumField(); i++ {
		if value := getTagValue(typeOfS.Field(i), tag); value != "" {
			values = append(values, value)
		}
	}
	return values
}

// GetStructFieldTagMap returns a map of tags using keyTag as map key and valueTag as map value
func GetStructFieldTagMap(Struct interface{}, keyTag string, valueTag string) map[string]string {
	v := reflect.ValueOf(Struct)
	typeOfS := v.Type()

	var fields = make(map[string]string)
	for i := 0; i < v.NumField(); i++ {
		fields[getTagValue(typeOfS.Field(i), keyTag)] = getTagValue(typeOfS.Field(i), valueTag)
	}
	return fields
}

// ScanRow is a cut-down version of the proposed Rows.ScanRow method. It
// currently only handles dest being a (pointer to) struct, and does not
// handle embedded fields. See https://github.com/golang/go/issues/61637
func ScanRow(rows *sql.Rows, columns []string, indexes map[string]int, dest any) error {
	// elem := reflect.ValueOf(dest).Elem()
	// if rv.Kind() != reflect.Pointer || rv.IsNil() {
	// 	return errors.New("dest must be a non-nil pointer")
	// }

	// elem := rv.Elem()
	// if elem.Kind() != reflect.Struct {
	// 	return errors.New("dest must point to a struct")
	// }

	var scanArgs []any
	for _, column := range columns {
		index, ok := indexes[column]
		if ok {
			// We have a column to field mapping, scan the value.
			field := reflect.ValueOf(dest).Elem().Field(index)
			scanArgs = append(scanArgs, field.Addr().Interface())
		}
	}
	return rows.Scan(scanArgs...)
}

// fieldIndexes returns a map of database column name to struct field index.
func fieldIndexes(structType reflect.Type) map[string]int {
	indexes := make(map[string]int)
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		tag := field.Tag.Get("sql")
		if tag != "" {
			// Use "sql" tag if set
			indexes[tag] = i
		} else {
			// Otherwise use field name (with exact case)
			indexes[field.Name] = i
		}
	}
	return indexes
}

// CachedFieldIndexes is like fieldIndexes, but cached per struct type.
func CachedFieldIndexes(structType reflect.Type) map[string]int {
	if f, ok := fieldIndexesCache.Load(structType); ok {
		return f.(map[string]int)
	}
	indexes := fieldIndexes(structType)
	fieldIndexesCache.Store(structType, indexes)
	return indexes
}
