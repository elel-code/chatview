package ui

import (
	"cmp"
	"slices"
	"strings"

	"chatview/client/internal/domain"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func messageDetail(message domain.Message) string {
	parts := make([]string, 0, 3)
	if message.Timestamp != "" {
		parts = append(parts, message.Timestamp)
	}
	if message.Delivery != "" {
		parts = append(parts, message.Delivery)
	}
	if message.Error != "" {
		parts = append(parts, message.Error)
	}
	return strings.Join(parts, "  ")
}

func sortMessages(messages []domain.Message) {
	slices.SortStableFunc(messages, func(left, right domain.Message) int {
		if left.ServerSeq > 0 && right.ServerSeq > 0 && left.ServerSeq != right.ServerSeq {
			return cmp.Compare(left.ServerSeq, right.ServerSeq)
		}
		if left.Timestamp != "" && right.Timestamp != "" && left.Timestamp != right.Timestamp {
			return cmp.Compare(left.Timestamp, right.Timestamp)
		}
		return cmp.Or(
			cmp.Compare(left.ServerSeq, right.ServerSeq),
			cmp.Compare(left.ID, right.ID),
		)
	})
}

func iconForImportance(importance widget.Importance) fyne.Resource {
	switch importance {
	case widget.DangerImportance:
		return theme.ErrorIcon()
	case widget.WarningImportance:
		return theme.WarningIcon()
	case widget.SuccessImportance:
		return theme.ConfirmIcon()
	default:
		return theme.InfoIcon()
	}
}

func truncateText(text string, limit int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

func shortKey(publicKey string) string {
	if len(publicKey) <= 12 {
		return publicKey
	}
	return publicKey[:8] + "..." + publicKey[len(publicKey)-4:]
}
