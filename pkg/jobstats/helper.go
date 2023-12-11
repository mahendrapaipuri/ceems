package jobstats

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var (
	nodelistRegExp = regexp.MustCompile(`(\[\d+\-\d+\])`)
)

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
