package storage

import (
	"strconv"
	"strings"
)

func (s *Store) Messages(peerKey string, limit int) ([]Message, error) {
	messages, _, _, err := s.History(peerKey, "", limit, "older")
	return messages, err
}

func (s *Store) History(peerKey string, cursor string, limit int, direction string) ([]Message, string, bool, error) {
	owner := s.ownerKey()
	var rows []messageRow
	if limit <= 0 {
		limit = 50
	}
	cursorSeq, hasCursor, err := parseCursor(cursor)
	if err != nil {
		return nil, "", false, err
	}
	direction = normalizeDirection(direction)
	query, args := historyQuery(owner, peerKey, cursorSeq, hasCursor, limit+1, direction)
	if err := s.db.Select(&rows, query, args...); err != nil {
		return nil, "", false, err
	}

	messages := mapSlice(rows, messageRow.message)
	hasMore := len(messages) > limit
	if hasMore {
		if direction == "newer" {
			messages = messages[:limit]
		} else {
			messages = messages[1:]
		}
	}
	nextCursor := ""
	if hasMore && len(messages) > 0 {
		if direction == "newer" {
			nextCursor = strconv.FormatInt(messages[len(messages)-1].ServerSeq, 10)
		} else {
			nextCursor = strconv.FormatInt(messages[0].ServerSeq, 10)
		}
	}
	return messages, nextCursor, hasMore, nil
}

func (s *Store) MaxServerSeq(peerKey string) (int64, error) {
	owner := s.ownerKey()
	var seq int64
	err := s.db.QueryRow(`
		SELECT COALESCE(MAX(server_seq), 0)
		FROM messages
		WHERE owner_pub_key = ? AND peer_pub_key = ? AND server_seq > 0
	`, owner, peerKey).Scan(&seq)
	return seq, err
}

func (s *Store) FirstServerSeqGap(peerKey string) (previousSeq int64, missingSeq int64, ok bool, err error) {
	owner := s.ownerKey()
	var rows []struct {
		ServerSeq int64 `db:"server_seq"`
	}
	if err := s.db.Select(&rows, `
		SELECT DISTINCT server_seq
		FROM messages
		WHERE owner_pub_key = ? AND peer_pub_key = ? AND server_seq > 0
		ORDER BY server_seq ASC
	`, owner, peerKey); err != nil {
		return 0, 0, false, err
	}
	var previous int64
	for _, row := range rows {
		if previous > 0 && row.ServerSeq > previous+1 {
			return previous, previous + 1, true, nil
		}
		previous = row.ServerSeq
	}
	return 0, 0, false, nil
}

func (s *Store) MessageByClientID(clientID string) (Message, error) {
	owner := s.ownerKey()
	var row messageRow
	if err := s.db.Get(&row, `
		SELECT id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq
		FROM messages
		WHERE owner_pub_key = ? AND (client_msg_id = ? OR id = ?)
		ORDER BY server_seq DESC, timestamp DESC
		LIMIT 1
	`, owner, clientID, clientID); err != nil {
		return Message{}, err
	}
	return row.message(), nil
}

func (s *Store) MarkConversationRead(peerKey string, seq int64) error {
	owner := s.ownerKey()
	_, err := s.db.Exec(`
		UPDATE friends
		SET unread = 0, last_seen_seq = MAX(last_seen_seq, ?), updated_at = ?
		WHERE owner_pub_key = ? AND pub_key = ?
	`, seq, nowRFC3339(), owner, peerKey)
	return err
}

func parseCursor(cursor string) (int64, bool, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, false, nil
	}
	seq, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil || seq < 0 {
		return 0, false, strconv.ErrSyntax
	}
	return seq, true, nil
}

func normalizeDirection(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "newer") {
		return "newer"
	}
	return "older"
}

const cachedMessageColumns = `id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq`

func historyQuery(owner, peerKey string, cursorSeq int64, hasCursor bool, limit int, direction string) (string, []any) {
	if direction == "newer" {
		if hasCursor {
			return `
				SELECT ` + cachedMessageColumns + `
				FROM messages
				WHERE owner_pub_key = ? AND peer_pub_key = ? AND server_seq > ?
				ORDER BY server_seq ASC, timestamp ASC
				LIMIT ?
			`, []any{owner, peerKey, cursorSeq, limit}
		}
		return `
			SELECT ` + cachedMessageColumns + `
			FROM messages
			WHERE owner_pub_key = ? AND peer_pub_key = ?
			ORDER BY server_seq ASC, timestamp ASC
			LIMIT ?
		`, []any{owner, peerKey, limit}
	}

	if hasCursor {
		return `
			SELECT ` + cachedMessageColumns + `
			FROM (
				SELECT ` + cachedMessageColumns + `
				FROM messages
				WHERE owner_pub_key = ? AND peer_pub_key = ? AND server_seq < ?
				ORDER BY server_seq DESC, timestamp DESC
				LIMIT ?
			)
			ORDER BY server_seq ASC, timestamp ASC
		`, []any{owner, peerKey, cursorSeq, limit}
	}
	return `
		SELECT ` + cachedMessageColumns + `
		FROM (
			SELECT ` + cachedMessageColumns + `
			FROM messages
			WHERE owner_pub_key = ? AND peer_pub_key = ?
			ORDER BY server_seq DESC, timestamp DESC
			LIMIT ?
		)
		ORDER BY server_seq ASC, timestamp ASC
	`, []any{owner, peerKey, limit}
}
