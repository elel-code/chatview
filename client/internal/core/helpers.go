package core

import (
	"crypto/rand"
	"slices"
	"time"

	"chatview/client/internal/domain"
)

func containsServerSeq(messages []domain.Message, seq int64) bool {
	return slices.ContainsFunc(messages, func(message domain.Message) bool {
		return message.ServerSeq == seq
	})
}

func maxServerSeq(messages []domain.Message) int64 {
	var seq int64
	for _, message := range messages {
		seq = max(seq, message.ServerSeq)
	}
	return seq
}

func syncLimit(expectedCount int32) int32 {
	if expectedCount <= 0 {
		return 100
	}
	return min(max(expectedCount, 30), 100)
}

func newClientMessageID() string {
	return rand.Text()
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func nextRetryTime(attempts int) string {
	delay := 5 * time.Second
	for i := 1; i < attempts; i++ {
		delay *= 2
		if delay > 5*time.Minute {
			delay = 5 * time.Minute
			break
		}
	}
	return time.Now().UTC().Add(delay).Format(time.RFC3339)
}

func mapSlice[S ~[]E, E any, T any](values S, fn func(E) T) []T {
	result := make([]T, 0, len(values))
	for _, value := range values {
		result = append(result, fn(value))
	}
	return result
}
