package ui

import (
	"fmt"
	"image/color"

	"chatview/client/internal/domain"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (a *Application) bindFriendListItem(id widget.ListItemID, item fyne.CanvasObject) {
	if id < 0 || id >= len(a.friends) {
		return
	}
	row := item.(*friendListItem)
	friend := a.friends[id]

	row.name.SetText(friendDisplayName(friend))
	row.detail.SetText(friendPresenceDetail(friend))
	row.onlineDot.FillColor = offlineDotColor
	if friend.Online {
		row.onlineDot.FillColor = onlineDotColor
	}
	row.onlineDot.Refresh()

	if friend.Unread == 0 {
		row.unreadText.Text = ""
		row.unreadText.Refresh()
		row.unreadBadge.Hide()
		return
	}
	row.unreadText.Text = fmt.Sprintf("%d", friend.Unread)
	row.unreadText.Refresh()
	row.unreadBadge.Show()
}

func (a *Application) bindMessageListItem(id widget.ListItemID, item fyne.CanvasObject) {
	if id < 0 || id >= len(a.messages) {
		return
	}
	row := item.(*messageListItem)
	message := a.messages[id]

	sender := shortKey(message.Sender)
	own := message.Sender == a.publicKey
	if own {
		sender = "You"
	}
	row.sender.SetText(sender)
	row.body.SetText(message.Text)
	row.detail.SetText(messageDetail(message))

	row.bubble.FillColor = incomingBubbleColor
	row.sender.Importance = widget.LowImportance
	row.leftSpacer.Hide()
	row.rightSpacer.Show()
	if own {
		row.bubble.FillColor = outgoingBubbleColor
		row.sender.Importance = widget.HighImportance
		row.leftSpacer.Show()
		row.rightSpacer.Hide()
	}
	row.bubble.Refresh()
	row.sender.Refresh()
	row.Container.Refresh()
}

func (a *Application) bindAdminListItem(id widget.ListItemID, item fyne.CanvasObject) {
	if id < 0 || id >= len(a.adminUsers) {
		return
	}
	row := item.(*adminListItem)
	user := a.adminUsers[id]

	row.key.SetText(user.PublicKey)
	state, stateColor := adminUserState(user)
	row.onlineDot.FillColor = offlineDotColor
	if user.Online {
		row.onlineDot.FillColor = onlineDotColor
	}
	row.onlineDot.Refresh()
	setTextBadge(row.stateBadge, state, stateColor)

	if user.Banned {
		row.action.SetText("Unban")
		row.action.SetIcon(theme.ConfirmIcon())
		row.action.Importance = widget.SuccessImportance
	} else {
		row.action.SetText("Ban")
		row.action.SetIcon(theme.CancelIcon())
		row.action.Importance = widget.DangerImportance
	}
	row.action.OnTapped = func() {
		a.confirmSetUserBanned(user.PublicKey, !user.Banned)
	}
}

func friendDisplayName(friend domain.Friend) string {
	if friend.Alias != "" {
		return friend.Alias
	}
	return shortKey(friend.PublicKey)
}

func friendPresenceDetail(friend domain.Friend) string {
	status := "offline"
	if friend.Online {
		status = "online"
	}
	return status + "  " + shortKey(friend.PublicKey)
}

func adminUserState(user domain.UserInfo) (string, color.Color) {
	if user.Banned {
		return "banned", errorColor
	}
	if user.Online {
		return "online active", successColor
	}
	return "offline active", mutedForegroundColor
}
