// Package helper provides utility functions across sub packages
package helper

import (
	"time"
)

// TimeToTimestamp converts a date in a given layout to unix timestamp of the date.
func TimeToTimestamp(layout string, date string) int64 {
	if t, err := time.Parse(layout, date); err == nil {
		return t.UnixMilli()
	}

	return 0
}

// ChunkBy splits the slice into chunks of given size.
func ChunkBy[T any](items []T, chunkSize int) [][]T {
	if chunkSize == 0 {
		return [][]T{items}
	}

	_chunks := make([][]T, 0, (len(items)/chunkSize)+1)
	for chunkSize < len(items) {
		items, _chunks = items[chunkSize:], append(_chunks, items[0:chunkSize:chunkSize])
	}

	return append(_chunks, items)
}
