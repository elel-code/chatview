package storage

type friendRow struct {
	PublicKey string `db:"pub_key"`
	Alias     string `db:"alias"`
	Online    int    `db:"is_online"`
	Unread    int32  `db:"unread"`
}

func (r friendRow) friend() Friend {
	return Friend{
		PublicKey: r.PublicKey,
		Alias:     r.Alias,
		Online:    r.Online != 0,
		Unread:    r.Unread,
	}
}

type messageRow struct {
	ID        string `db:"id"`
	ClientID  string `db:"client_msg_id"`
	PeerKey   string `db:"peer_pub_key"`
	Sender    string `db:"sender_pub_key"`
	Text      string `db:"text"`
	Timestamp string `db:"timestamp"`
	Delivery  string `db:"delivery"`
	Error     string `db:"error"`
	ServerSeq int64  `db:"server_seq"`
}

func (r messageRow) message() Message {
	return Message{
		ID:        r.ID,
		ClientID:  r.ClientID,
		PeerKey:   r.PeerKey,
		Sender:    r.Sender,
		Text:      r.Text,
		Timestamp: r.Timestamp,
		Delivery:  r.Delivery,
		Error:     r.Error,
		ServerSeq: r.ServerSeq,
	}
}

type outboxRow struct {
	ID          string `db:"id"`
	ReceiverKey string `db:"receiver_pub_key"`
	Text        string `db:"text"`
	Attempts    int    `db:"attempts"`
	NextRetryAt string `db:"next_retry_at"`
	Error       string `db:"error"`
	Status      int    `db:"status"`
	CreatedAt   string `db:"created_at"`
}

func (r outboxRow) item() OutboxItem {
	return OutboxItem{
		ID:          r.ID,
		ReceiverKey: r.ReceiverKey,
		Text:        r.Text,
		Attempts:    r.Attempts,
		NextRetryAt: r.NextRetryAt,
		Error:       r.Error,
		Status:      r.Status,
		CreatedAt:   r.CreatedAt,
	}
}

type outboxMessageRow struct {
	ReceiverKey string `db:"receiver_pub_key"`
	Text        string `db:"text"`
}
