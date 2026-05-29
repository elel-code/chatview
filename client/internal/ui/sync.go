package ui

import (
	"context"
	"fmt"
	"strings"

	"chatview/client/internal/core"

	"fyne.io/fyne/v2"
)

func (a *Application) refreshFriends() {
	if a.refreshing {
		return
	}
	a.refreshing = true
	version := a.sessionVersion
	ctx := a.sessionContext()
	a.setStatus("refreshing friends...")
	go func() {
		friends, err := a.service.ListFriends(ctx)
		fyne.Do(func() {
			a.refreshing = false
			if ctx.Err() != nil || !a.isCurrentSession(version) || a.friendList == nil {
				return
			}
			if err != nil {
				a.setStatus(err.Error())
				return
			}
			a.friends = friends
			a.friendList.Refresh()
			a.reselectFriend()
			a.updateEmptyStates()
			a.refreshOutboxStatus()
			a.setStatus(fmt.Sprintf("%d friends", len(friends)))
			if !a.offline {
				a.syncUnreadConversations(friends)
			}
		})
	}()
}

func (a *Application) loadHistory(publicKey string) {
	a.setChatTitle(publicKey)
	a.historyRequest++
	request := a.historyRequest
	version := a.sessionVersion
	ctx := a.sessionContext()
	a.setStatus("loading messages...")
	a.historyLoading = true
	a.updateHistoryState()
	go func() {
		page, err := a.service.GetHistory(ctx, publicKey, "", 50, "older")
		fyne.Do(func() {
			if ctx.Err() != nil || !a.isCurrentSession(version) || request != a.historyRequest || !a.isSelectedConversation(publicKey) {
				return
			}
			a.historyLoading = false
			if err != nil {
				a.setStatus(err.Error())
				a.updateHistoryState()
				return
			}
			a.replaceHistory(page)
			a.refreshMessageView(false)
			a.updateHistoryState()
			if len(a.messages) > 0 {
				a.scrollMessagesToBottom()
				a.markConversationReadThroughLastMessage(ctx, publicKey)
			}
			a.refreshOutboxStatus()
			a.updateMessageCountStatus()
		})
	}()
}

func (a *Application) loadOlderHistory() {
	if a.selectedFriend == nil {
		a.setStatus("select a friend first")
		return
	}
	if a.historyLoading {
		return
	}
	if !a.historyHasMore {
		a.setStatus("oldest message reached")
		return
	}
	a.historyLoading = true
	a.updateHistoryState()
	a.setStatus("loading older messages...")
	peer := a.selectedFriend.PublicKey
	cursor := a.historyCursor
	request := a.historyRequest
	version := a.sessionVersion
	ctx := a.sessionContext()
	go func() {
		page, err := a.service.GetHistory(ctx, peer, cursor, 50, "older")
		fyne.Do(func() {
			if ctx.Err() != nil || !a.isCurrentSession(version) || request != a.historyRequest || !a.isSelectedConversation(peer) {
				return
			}
			a.historyLoading = false
			if err != nil {
				a.setStatus(err.Error())
				a.updateHistoryState()
				return
			}
			a.prependHistory(page)
			a.refreshMessageView(false)
			a.updateHistoryState()
			a.refreshOutboxStatus()
			a.updateMessageCountStatus()
		})
	}()
}

func (a *Application) sendMessage() {
	if a.sending {
		return
	}
	if a.selectedFriend == nil {
		a.setStatus("select a friend first")
		return
	}
	text := strings.TrimSpace(a.messageBox.Text)
	if text == "" {
		a.setStatus("message is required")
		return
	}
	receiver := a.selectedFriend.PublicKey
	a.messageBox.SetText("")
	a.sending = true
	a.updateComposerState()
	version := a.sessionVersion
	ctx := a.sessionContext()
	a.setStatus("sending...")
	go func() {
		message, err := a.service.SendMessage(ctx, receiver, text)
		fyne.Do(func() {
			a.sending = false
			a.updateComposerState()
			if ctx.Err() != nil || !a.isCurrentSession(version) {
				return
			}
			if err != nil {
				a.messageBox.SetText(text)
				a.setStatus(err.Error())
				return
			}
			a.upsertMessage(message)
			a.refreshMessageView(true)
			a.refreshOutboxStatus()
			a.refreshFriends()
			a.setStatus(message.Delivery)
		})
	}()
}

func (a *Application) syncConversation(publicKey string, expectedCount int32) {
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" {
		return
	}
	version := a.sessionVersion
	request := a.historyRequest
	ctx := a.sessionContext()
	go func() {
		page, err := a.service.SyncConversation(ctx, publicKey, expectedCount)
		fyne.Do(func() {
			if ctx.Err() != nil || !a.isCurrentSession(version) {
				return
			}
			if err != nil {
				a.setStatus(err.Error())
				return
			}
			if !a.isSelectedConversation(publicKey) || request != a.historyRequest {
				return
			}
			if len(page.Messages) == 0 {
				return
			}
			added := a.mergeMessages(page.Messages)
			a.refreshMessageView(true)
			a.refreshOutboxStatus()
			a.markConversationReadThroughLastMessage(ctx, publicKey)
			a.updateMessageCountStatus()
			if added > 0 {
				a.refreshFriends()
			}
		})
	}()
}

func (a *Application) syncUnreadConversations(friends []core.Friend) {
	if a.offline || a.syncingUnread {
		return
	}
	peers := friendsWithUnread(friends)
	if len(peers) == 0 {
		return
	}
	a.syncingUnread = true
	version := a.sessionVersion
	ctx := a.sessionContext()
	go func() {
		defer fyne.Do(func() {
			if a.sessionVersion == version {
				a.syncingUnread = false
			}
		})
		for _, peer := range peers {
			if ctx.Err() != nil {
				return
			}
			page, err := a.service.SyncConversation(ctx, peer.PublicKey, peer.Unread)
			fyne.Do(func() {
				if ctx.Err() != nil || !a.isCurrentSession(version) {
					return
				}
				if err != nil {
					a.setStatus(err.Error())
					return
				}
				if !a.isSelectedConversation(peer.PublicKey) || len(page.Messages) == 0 {
					return
				}
				added := a.mergeMessages(page.Messages)
				if added == 0 {
					return
				}
				a.refreshMessageView(true)
				a.refreshOutboxStatus()
				a.markConversationReadThroughLastMessage(ctx, peer.PublicKey)
				a.updateMessageCountStatus()
			})
		}
	}()
}

func (a *Application) replaceHistory(page core.HistoryPage) {
	a.messages = page.Messages
	a.historyCursor = page.NextCursor
	a.historyHasMore = page.HasMore
}

func (a *Application) prependHistory(page core.HistoryPage) {
	a.messages = append(page.Messages, a.messages...)
	a.historyCursor = page.NextCursor
	a.historyHasMore = page.HasMore
}

func (a *Application) refreshMessageView(scrollToBottom bool) {
	if a.messageList != nil {
		a.messageList.Refresh()
	}
	a.updateEmptyStates()
	if scrollToBottom {
		a.scrollMessagesToBottom()
	}
}

func (a *Application) scrollMessagesToBottom() {
	if a.messageList != nil {
		a.messageList.ScrollToBottom()
	}
}

func (a *Application) updateMessageCountStatus() {
	a.setStatus(fmt.Sprintf("%d messages", len(a.messages)))
}

func (a *Application) markConversationReadThroughLastMessage(ctx context.Context, publicKey string) {
	if len(a.messages) == 0 {
		return
	}
	lastSeq := a.messages[len(a.messages)-1].ServerSeq
	go func() {
		_ = a.service.MarkConversationRead(ctx, publicKey, lastSeq)
	}()
}

func friendsWithUnread(friends []core.Friend) []core.Friend {
	peers := make([]core.Friend, 0, len(friends))
	for _, friend := range friends {
		if friend.Unread > 0 && strings.TrimSpace(friend.PublicKey) != "" {
			peers = append(peers, friend)
		}
	}
	return peers
}
