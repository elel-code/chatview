package domain

type LoginResult struct {
	PublicKey string `json:"publicKey"`
	Role      int32  `json:"role"`
	Offline   bool   `json:"offline"`
}

type Friend struct {
	PublicKey string `json:"publicKey"`
	Alias     string `json:"alias"`
	Online    bool   `json:"online"`
	Unread    int32  `json:"unread"`
}

type Message struct {
	ID        string `json:"id"`
	Sender    string `json:"sender"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
	Delivery  string `json:"delivery"`
	Error     string `json:"error"`
	ServerSeq int64  `json:"serverSeq"`
}

type HistoryPage struct {
	Messages   []Message `json:"messages"`
	NextCursor string    `json:"nextCursor"`
	HasMore    bool      `json:"hasMore"`
}

type SendResult struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	ServerSeq int64  `json:"serverSeq"`
}

type Event struct {
	Kind      string `json:"kind"`
	PublicKey string `json:"publicKey"`
	Text      string `json:"text"`
	Reason    string `json:"reason"`
	Count     int32  `json:"count"`
}

type AdminStats struct {
	OnlineUsers int32 `json:"onlineUsers"`
	TotalUsers  int32 `json:"totalUsers"`
	BannedUsers int32 `json:"bannedUsers"`
}

type UserInfo struct {
	PublicKey string `json:"publicKey"`
	Online    bool   `json:"online"`
	Banned    bool   `json:"banned"`
}

type AdminUpdate struct {
	Users []UserInfo `json:"users"`
	Stats AdminStats `json:"stats"`
}

type OutboxStatus struct {
	Pending int `json:"pending"`
	Failed  int `json:"failed"`
}

type AuthLockState struct {
	LockedUntil       string `json:"lockedUntil"`
	RemainingAttempts int    `json:"remainingAttempts"`
}
