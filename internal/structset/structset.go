// Package structset implements helper functions that involves structs
package structset

import (
	"database/sql"
	"reflect"
	"strings"
	"sync"
)

var fieldIndexesCache sync.Map

// StructFieldNames returns all fields in a given struct.
func StructFieldNames(s any) []string {
	v := reflect.ValueOf(s)
	typeOfS := v.Type()

	fields := make([]string, v.NumField())

	for i := range v.NumField() {
		fields[i] = typeOfS.Field(i).Name
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

// tagValue returns tag value of field. If tag value is "-", empty string will be returned
// If tag is empty, return name of field.
func tagValue(field reflect.StructField, tag string) string {
	switch field.Tag.Get(tag) {
	case "-":
		return ""
	case "":
		return field.Name
	default:
		return strings.Split(field.Tag.Get(tag), ",")[0]
	}
}

// StructFieldTagValues returns all tag names in a given struct for a given tag.
func StructFieldTagValues(s any, tag string) []string {
	v := reflect.ValueOf(s)
	typeOfS := v.Type()

	var values []string

	for i := range v.NumField() {
		if value := tagValue(typeOfS.Field(i), tag); value != "" {
			values = append(values, value)
		}
	}

	return values
}

// StructFieldTagMap returns a map of tags using keyTag as map key and valueTag as map value.
func StructFieldTagMap(s any, keyTag string, valueTag string) map[string]string {
	v := reflect.ValueOf(s)
	typeOfS := v.Type()

	fields := make(map[string]string)
	for i := range v.NumField() {
		fields[tagValue(typeOfS.Field(i), keyTag)] = tagValue(typeOfS.Field(i), valueTag)
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

	for i := range structType.NumField() {
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
		if m, mOk := f.(map[string]int); mOk {
			return m
		}
	}

	indexes := fieldIndexes(structType)
	fieldIndexesCache.Store(structType, indexes)

	return indexes
}
