package storage

import "time"

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func mapSlice[S ~[]E, E any, T any](values S, fn func(E) T) []T {
	result := make([]T, 0, len(values))
	for _, value := range values {
		result = append(result, fn(value))
	}
	return result
}
