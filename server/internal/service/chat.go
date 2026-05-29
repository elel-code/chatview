package service

import (
	chatpb "chatview/api/gen/chatview/chat"
	"chatview/server/internal/db"
	"chatview/server/internal/eventhub"
)

type ChatService struct {
	chatpb.UnimplementedChatServiceServer
	Store *db.Store
	Hub   *eventhub.Hub
}
