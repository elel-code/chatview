package rpcclient

type Options struct {
	Target                string
	UseTLS                bool
	CACertPath            string
	SSLTargetNameOverride string
}

type LoginResult struct {
	PublicKey string
	Role      int32
}

type Friend struct {
	PublicKey string
	Alias     string
	Online    bool
	Unread    int32
}

type Message struct {
	ID        string
	Sender    string
	Text      string
	Timestamp string
	Delivery  string
	Error     string
	ServerSeq int64
}

type HistoryPage struct {
	Messages   []Message
	NextCursor string
	HasMore    bool
}

type SendResult struct {
	ID        string
	Timestamp string
	ServerSeq int64
}

type Event struct {
	Kind      string
	PublicKey string
	Text      string
	Reason    string
	Count     int32
}

type AdminStats struct {
	OnlineUsers int32
	TotalUsers  int32
	BannedUsers int32
}

type UserInfo struct {
	PublicKey string
	Online    bool
	Banned    bool
}

type AdminUpdate struct {
	Users []UserInfo
	Stats AdminStats
}
