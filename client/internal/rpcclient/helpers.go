package rpcclient

import "crypto/rand"

func randomMessageID() string {
	return rand.Text()
}

func mapSlice[S ~[]E, E any, T any](values S, fn func(E) T) []T {
	result := make([]T, 0, len(values))
	for _, value := range values {
		result = append(result, fn(value))
	}
	return result
}
