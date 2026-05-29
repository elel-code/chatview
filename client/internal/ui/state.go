package ui

import (
	"context"
	"strings"

	"chatview/client/internal/domain"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (a *Application) upsertMessage(message domain.Message) {
	for i := range a.messages {
		if a.messages[i].ID == message.ID {
			a.messages[i] = message
			return
		}
	}
	a.messages = append(a.messages, message)
}

func (a *Application) mergeMessages(messages []domain.Message) int {
	added := 0
	for _, message := range messages {
		before := len(a.messages)
		a.upsertMessage(message)
		if len(a.messages) > before {
			added++
		}
	}
	sortMessages(a.messages)
	return added
}

func (a *Application) resetSessionState() {
	a.stopEvents()
	a.stopOutboxPoller()
	a.stopSessionContext()
	a.sessionVersion++
	if a.noticePopup != nil {
		a.noticePopup.Hide()
		a.noticePopup = nil
	}
	a.friends = nil
	a.messages = nil
	a.adminUsers = nil
	a.selectedFriend = nil
	a.publicKey = ""
	a.offline = false
	a.historyCursor = ""
	a.historyHasMore = false
	a.historyLoading = false
	a.updateHistoryState()
	a.selectingFriend = false
	a.loginInFlight = false
	a.sending = false
	a.refreshing = false
	a.refreshingAdmin = false
	a.syncingUnread = false
	a.lastOutbox = domain.OutboxStatus{}
}

func (a *Application) startSessionContext() {
	a.stopSessionContext()
	a.sessionCtx, a.sessionCancel = context.WithCancel(context.Background())
}

func (a *Application) stopSessionContext() {
	if a.sessionCancel != nil {
		a.sessionCancel()
		a.sessionCancel = nil
	}
	a.sessionCtx = nil
}

func (a *Application) sessionContext() context.Context {
	if a.sessionCtx != nil {
		return a.sessionCtx
	}
	return context.Background()
}

func (a *Application) isCurrentSession(version int) bool {
	return a.sessionVersion == version && a.publicKey != ""
}

func (a *Application) isSelectedConversation(publicKey string) bool {
	return a.selectedFriend != nil && a.selectedFriend.PublicKey == publicKey
}

func (a *Application) selectConversation(publicKey string) {
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" || a.friendList == nil {
		return
	}
	for i := range a.friends {
		if a.friends[i].PublicKey != publicKey {
			continue
		}
		friend := a.friends[i]
		a.selectedFriend = &friend
		a.selectingFriend = true
		a.friendList.Select(i)
		a.selectingFriend = false
		a.updateComposerState()
		a.loadHistory(publicKey)
		return
	}
	a.refreshFriends()
}

func (a *Application) reselectFriend() {
	if a.selectedFriend == nil || a.friendList == nil {
		return
	}
	selectedKey := a.selectedFriend.PublicKey
	for i := range a.friends {
		if a.friends[i].PublicKey == selectedKey {
			friend := a.friends[i]
			a.selectedFriend = &friend
			a.selectingFriend = true
			a.friendList.Select(i)
			a.selectingFriend = false
			a.updateEmptyStates()
			a.updateComposerState()
			a.updateHistoryState()
			return
		}
	}
	a.selectedFriend = nil
	a.messages = nil
	a.historyCursor = ""
	a.historyHasMore = false
	a.historyLoading = false
	a.setChatTitle("")
	if a.messageList != nil {
		a.messageList.Refresh()
	}
	a.updateEmptyStates()
	a.updateComposerState()
	a.updateHistoryState()
}

func (a *Application) setChatTitle(publicKey string) {
	if a.chatTitle == nil {
		return
	}
	if publicKey == "" {
		a.chatTitle.SetText("Select a conversation")
		return
	}
	label := shortKey(publicKey)
	for _, friend := range a.friends {
		if friend.PublicKey == publicKey && friend.Alias != "" {
			label = friend.Alias
			break
		}
	}
	a.chatTitle.SetText(label)
}

func (a *Application) setStatus(text string) {
	if a.status != nil {
		a.status.SetText(text)
	}
}

func (a *Application) statusBar() fyne.CanvasObject {
	return softBlock(container.NewBorder(nil, nil, widget.NewIcon(theme.InfoIcon()), nil, a.status))
}

func (a *Application) updateEmptyStates() {
	if a.friendEmpty != nil {
		if len(a.friends) == 0 {
			a.friendEmpty.Show()
		} else {
			a.friendEmpty.Hide()
		}
	}
	if a.messageEmpty != nil {
		if a.selectedFriend == nil {
			a.messageEmpty.SetText("Select a conversation")
			a.messageEmpty.Show()
		} else if len(a.messages) == 0 {
			a.messageEmpty.SetText("No messages")
			a.messageEmpty.Show()
		} else {
			a.messageEmpty.Hide()
		}
	}
}

func (a *Application) updateAdminEmpty() {
	if a.adminEmpty == nil {
		return
	}
	if len(a.adminUsers) == 0 {
		a.adminEmpty.Show()
		return
	}
	a.adminEmpty.Hide()
}

func (a *Application) updateComposerState() {
	if a.messageBox == nil || a.sendButton == nil {
		return
	}
	if a.selectedFriend == nil {
		a.messageBox.Disable()
		a.sendButton.Disable()
		return
	}
	a.messageBox.Enable()
	if a.sending {
		a.sendButton.Disable()
		return
	}
	if strings.TrimSpace(a.messageBox.Text) == "" {
		a.sendButton.Disable()
		return
	}
	a.sendButton.Enable()
}

func (a *Application) updateHistoryState() {
	if a.loadOlder == nil {
		return
	}
	if a.selectedFriend == nil || a.historyLoading || !a.historyHasMore {
		a.loadOlder.Disable()
		return
	}
	a.loadOlder.Enable()
}

func (a *Application) setContent(content fyne.CanvasObject) {
	a.window.SetContent(appFrame(content))
}
