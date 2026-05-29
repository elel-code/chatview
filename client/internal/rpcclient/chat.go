package rpcclient

import (
	"context"

	chatpb "chatview/api/gen/chatview/chat"
)

func (c *Client) ListFriends(ctx context.Context) ([]Friend, error) {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	resp, err := c.chat.ListFriends(ctx, &chatpb.ListFriendsReq{})
	if err != nil {
		return nil, rpcError(err)
	}
	return mapSlice(resp.Friends, friendFromProto), nil
}

func (c *Client) AddFriend(ctx context.Context, publicKey string) error {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	_, err := c.chat.AddFriend(ctx, &chatpb.AddFriendReq{TargetPubKey: publicKey})
	return rpcError(err)
}

func (c *Client) GetHistory(ctx context.Context, peerPublicKey string, cursor string, limit int32, direction string) (HistoryPage, error) {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	resp, err := c.chat.GetMessageHistory(ctx, &chatpb.GetMessageHistoryReq{
		PeerPubKey: peerPublicKey,
		Cursor:     cursor,
		Limit:      limit,
		Direction:  direction,
	})
	if err != nil {
		return HistoryPage{}, rpcError(err)
	}
	if resp.Page == nil {
		return HistoryPage{}, nil
	}
	return HistoryPage{
		Messages:   mapSlice(resp.Page.Messages, messageFromProto),
		NextCursor: resp.Page.NextCursor,
		HasMore:    resp.Page.HasMore,
	}, nil
}

func (c *Client) SendMessage(ctx context.Context, receiverPublicKey string, text string) (SendResult, error) {
	return c.SendMessageWithID(ctx, receiverPublicKey, text, randomMessageID())
}

func (c *Client) SendMessageWithID(ctx context.Context, receiverPublicKey string, text string, clientMessageID string) (SendResult, error) {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	resp, err := c.chat.SendMessage(ctx, &chatpb.SendMessageReq{
		ReceiverPubKey:  receiverPublicKey,
		Text:            text,
		ClientMessageId: clientMessageID,
	})
	if err != nil {
		return SendResult{}, rpcError(err)
	}
	return SendResult{
		ID:        resp.MessageId,
		Timestamp: resp.Timestamp,
		ServerSeq: resp.ServerSeq,
	}, nil
}

func (c *Client) MarkConversationRead(ctx context.Context, peerPublicKey string, seq int64) error {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	_, err := c.chat.MarkConversationRead(ctx, &chatpb.MarkConversationReadReq{
		PeerPubKey:        peerPublicKey,
		LastReadServerSeq: seq,
	})
	return rpcError(err)
}
