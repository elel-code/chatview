package storage

func (s *Store) EnqueueOutbox(item OutboxItem, senderKey string) error {
	owner := s.ownerKey()
	if item.CreatedAt == "" {
		item.CreatedAt = nowRFC3339()
	}
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		INSERT INTO outbox(owner_pub_key, id, receiver_pub_key, text, attempts, next_retry_at, error, status, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(owner_pub_key, id) DO UPDATE SET
			receiver_pub_key = excluded.receiver_pub_key,
			text = excluded.text,
			attempts = excluded.attempts,
			next_retry_at = excluded.next_retry_at,
			error = excluded.error,
			status = excluded.status,
			updated_at = excluded.updated_at
	`, owner, item.ID, item.ReceiverKey, item.Text, item.Attempts, item.NextRetryAt, item.Error, item.Status, item.CreatedAt, item.CreatedAt); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO messages(owner_pub_key, id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)
		ON CONFLICT(owner_pub_key, id) DO UPDATE SET
			delivery = excluded.delivery,
			error = excluded.error,
			updated_at = excluded.updated_at
	`, owner, item.ID, item.ID, item.ReceiverKey, senderKey, item.Text, item.CreatedAt, "pending", item.Error, item.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DueOutbox(now string, limit int) ([]OutboxItem, error) {
	owner := s.ownerKey()
	if limit <= 0 {
		limit = 20
	}
	var rows []outboxRow
	if err := s.db.Select(&rows, `
		SELECT id, receiver_pub_key, text, attempts, next_retry_at, error, status, created_at
		FROM outbox
		WHERE owner_pub_key = ? AND status = 0 AND (next_retry_at = '' OR next_retry_at <= ?)
		ORDER BY created_at
		LIMIT ?
	`, owner, now, limit); err != nil {
		return nil, err
	}

	return mapSlice(rows, outboxRow.item), nil
}

func (s *Store) MarkOutboxSending(id string) error {
	owner := s.ownerKey()
	_, err := s.db.Exec(`UPDATE outbox SET status = 1, error = '', updated_at = ? WHERE owner_pub_key = ? AND id = ?`, nowRFC3339(), owner, id)
	return err
}

func (s *Store) MarkOutboxSent(clientID string, serverID string, timestamp string, serverSeq int64, senderKey string) error {
	owner := s.ownerKey()
	if timestamp == "" {
		timestamp = nowRFC3339()
	}
	now := nowRFC3339()
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var row outboxMessageRow
	if err := tx.Get(&row, `SELECT receiver_pub_key, text FROM outbox WHERE owner_pub_key = ? AND id = ?`, owner, clientID); err != nil {
		return err
	}
	_, _ = tx.Exec(`DELETE FROM messages WHERE owner_pub_key = ? AND id = ?`, owner, clientID)
	if _, err := tx.Exec(`
		INSERT INTO messages(owner_pub_key, id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, 'sent', '', ?, ?)
		ON CONFLICT(owner_pub_key, id) DO UPDATE SET
			delivery = 'sent',
			error = '',
			server_seq = excluded.server_seq,
			updated_at = excluded.updated_at
	`, owner, serverID, clientID, row.ReceiverKey, senderKey, row.Text, timestamp, serverSeq, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM outbox WHERE owner_pub_key = ? AND id = ?`, owner, clientID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MarkOutboxRetry(id string, attempts int, nextRetryAt string, message string) error {
	owner := s.ownerKey()
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := nowRFC3339()
	if _, err := tx.Exec(`UPDATE outbox SET attempts = ?, next_retry_at = ?, error = ?, status = 0, updated_at = ? WHERE owner_pub_key = ? AND id = ?`, attempts, nextRetryAt, message, now, owner, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE messages SET delivery = 'pending', error = ?, updated_at = ? WHERE owner_pub_key = ? AND (client_msg_id = ? OR id = ?)`, message, now, owner, id, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MarkOutboxFailed(id string, attempts int, message string) error {
	owner := s.ownerKey()
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := nowRFC3339()
	if _, err := tx.Exec(`UPDATE outbox SET attempts = ?, next_retry_at = '', error = ?, status = 2, updated_at = ? WHERE owner_pub_key = ? AND id = ?`, attempts, message, now, owner, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE messages SET delivery = 'failed', error = ?, updated_at = ? WHERE owner_pub_key = ? AND (client_msg_id = ? OR id = ?)`, message, now, owner, id, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) OutboxStatus() (pending int, failed int, err error) {
	owner := s.ownerKey()
	err = s.db.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN status IN (0, 1) THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 2 THEN 1 ELSE 0 END), 0)
		FROM outbox
		WHERE owner_pub_key = ?
	`, owner).Scan(&pending, &failed)
	if err != nil {
		return 0, 0, err
	}
	return pending, failed, nil
}

func (s *Store) RetryFailedOutbox() error {
	owner := s.ownerKey()
	_, err := s.db.Exec(`UPDATE outbox SET status = 0, next_retry_at = '', error = '', updated_at = ? WHERE owner_pub_key = ? AND status = 2`, nowRFC3339(), owner)
	return err
}

func (s *Store) ClearFailedOutbox() error {
	owner := s.ownerKey()
	_, err := s.db.Exec(`
		DELETE FROM messages WHERE owner_pub_key = ? AND id IN (SELECT id FROM outbox WHERE owner_pub_key = ? AND status = 2);
		DELETE FROM outbox WHERE owner_pub_key = ? AND status = 2;
	`, owner, owner, owner)
	return err
}

func (s *Store) RecoverOutbox() error {
	owner := s.ownerKey()
	_, err := s.db.Exec(`UPDATE outbox SET status = 0, next_retry_at = '', updated_at = ? WHERE owner_pub_key = ? AND status = 1`, nowRFC3339(), owner)
	return err
}
