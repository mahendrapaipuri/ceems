package helpers

import (
	"strings"

	"github.com/google/uuid"
	"github.com/zeebo/xxh3"
)

// Get a UUID5 for given slice of strings
func GetUUIDFromString(stringSlice []string) (string, error) {
	s := strings.Join(stringSlice[:], ",")
	h := xxh3.HashString128(s).Bytes()
	uuid, err := uuid.FromBytes(h[:])
	return uuid.String(), err
}
