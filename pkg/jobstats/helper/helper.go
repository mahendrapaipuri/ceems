package helper

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	nodelistRegExp = regexp.MustCompile(`\[(.*?)\]`)
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

// Replace delimiter in nodelist
// The default delimiter "," is used to separate nodes and ranges. So we first
// replace the delimiter of nodes to | and call parser function
func replaceNodelistDelimiter(nodelistExp string) string {
	// Split expression into slice
	// This will split both nodes and ranges
	// Eg a[0-1,3,5-6],b[2-3,4] will be split into "a[0-1", "3", "5-6]", "b[2-3", "4]"
	// We need the rejoin the resulting slice to get node ranges together
	var nodelistExpSlice = strings.Split(nodelistExp, ",")
	var nodelist []string
	var idxEnd int = 0
	for idx, nodeexp := range nodelistExpSlice {
		// If string contains only "[", it was split in the range as well
		if strings.Contains(nodeexp, "[") && !strings.Contains(nodeexp, "]") {
			idxEnd = idx
			// Keep matching until we find "]" and not "["
			for {
				idxEnd++
				if strings.Contains(nodelistExpSlice[idxEnd], "]") && !strings.Contains(nodelistExpSlice[idxEnd], "[") {
					break
				}
			}
			nodelist = append(nodelist, strings.Join(nodelistExpSlice[idx:idxEnd+1], ","))
		} else if idx != 0 && idx <= idxEnd {
			// Ignore all the indices that we already sweeped in above loop
			continue
		} else {
			idxEnd = idx
			nodelist = append(nodelist, nodeexp)
		}
	}
	return strings.Join(nodelist, "|")
}

// Expand nodelist range string into slice of node names recursively
func expandNodelist(nodelistExp string) []string {
	var nodeNames []string
	// First split by | to get individual nodes
	for _, nodeexp := range strings.Split(nodelistExp, "|") {
		if strings.Contains(nodeexp, "[") {
			matches := nodelistRegExp.FindAllString(nodeexp, -1)
			if len(matches) == 0 {
				continue
			}

			// Get only first match as we use recursion
			for _, match := range matches[0:1] {
				matchSansBrackets := match[1 : len(match)-1]
				// matchSansBranckets can have multiple ranges like 0-2,4,5-8
				// Split them by ","
				for _, subMatches := range strings.Split(matchSansBrackets, ",") {
					subMatch := strings.Split(subMatches, "-")
					// If subMatch is single number, copy it as second index
					if len(subMatch) == 1 {
						subMatch = append(subMatch, subMatch[0])
					}

					// Convert strings into ints
					startIdx, err := strconv.Atoi(subMatch[0])
					if err != nil {
						continue
					}
					endIdx, err := strconv.Atoi(subMatch[1])
					if err != nil {
						continue
					}

					// Append node names to slice
					for i := startIdx; i <= endIdx; i++ {
						nodename := strings.Replace(nodeexp, match, strconv.Itoa(i), -1)
						// Add them to slice and call function again
						nodeNames = append(nodeNames, expandNodelist(nodename)...)
					}
				}
			}
		} else {
			nodeNames = append(nodeNames, regexp.QuoteMeta(nodeexp))
		}
	}
	return nodeNames
}

// Expand SLURM NODELIST into slice of nodenames
func NodelistParser(nodelistExp string) []string {
	return expandNodelist(replaceNodelistDelimiter(nodelistExp))
}

// Converts a date in a given layout to unix timestamp of the date
func TimeToTimestamp(layout string, date string) int64 {
	if t, err := time.Parse(layout, date); err == nil {
		return int64(t.Local().UnixMilli())
	}
	return 0
}
