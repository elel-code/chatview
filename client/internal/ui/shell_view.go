package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type shellButtons struct {
	refresh   *widget.Button
	addFriend *widget.Button
	reconnect *widget.Button
	copyKey   *widget.Button
	exportKey *widget.Button
	logout    *widget.Button
}

func (a *Application) showShell(role int32) {
	a.prepareShellState()
	chatView := a.buildChatShell()
	a.updateEmptyStates()
	a.updateComposerState()
	a.updateHistoryState()

	content := fyne.CanvasObject(chatView)
	if role == 1 {
		content = container.NewAppTabs(
			container.NewTabItem("Chat", chatView),
			container.NewTabItem("Admin", a.buildAdminView()),
		)
	}
	a.setContent(container.NewBorder(nil, a.statusBar(), nil, nil, content))
	a.refreshOutboxStatus()
	a.startOutboxPoller()
}

func (a *Application) prepareShellState() {
	a.sessionVersion++
	a.syncingUnread = false
	a.friendEmpty = emptyLabel("No conversations")
	a.messageEmpty = emptyLabel("Select a conversation")
}

func (a *Application) buildChatShell() fyne.CanvasObject {
	a.friendList = a.newFriendList()
	a.messageList = a.newMessageList()
	a.buildComposer()
	buttons := a.buildShellButtons()

	chatView := container.NewHSplit(
		panel(a.buildSidebar(buttons)),
		panel(a.buildChatPanel(buttons.logout)),
	)
	chatView.SetOffset(0.28)
	return chatView
}

func (a *Application) newFriendList() *widget.List {
	list := widget.NewList(
		func() int { return len(a.friends) },
		func() fyne.CanvasObject {
			return newFriendListItem()
		},
		a.bindFriendListItem,
	)
	list.OnSelected = func(id widget.ListItemID) {
		if a.selectingFriend {
			return
		}
		if id < 0 || id >= len(a.friends) {
			return
		}
		friend := a.friends[id]
		a.selectedFriend = &friend
		a.updateComposerState()
		a.loadHistory(friend.PublicKey)
	}
	return list
}

func (a *Application) newMessageList() *widget.List {
	return widget.NewList(
		func() int { return len(a.messages) },
		func() fyne.CanvasObject {
			return newMessageListItem()
		},
		a.bindMessageListItem,
	)
}

func (a *Application) buildComposer() {
	a.messageBox = widget.NewMultiLineEntry()
	a.messageBox.SetPlaceHolder("Message")
	a.messageBox.Wrapping = fyne.TextWrapWord
	a.messageBox.SetMinRowsVisible(3)
	a.messageBox.OnSubmitted = func(_ string) {
		a.sendMessage()
	}
	a.messageBox.OnChanged = func(_ string) {
		a.updateComposerState()
	}
	a.messageBox.Disable()

	a.chatTitle = widget.NewLabel("Select a conversation")
	a.chatTitle.TextStyle = fyne.TextStyle{Bold: true}
	a.sendButton = widget.NewButtonWithIcon("Send", theme.MailSendIcon(), func() {
		a.sendMessage()
	})
	a.sendButton.Importance = widget.HighImportance
	a.sendButton.Disable()
	a.loadOlder = widget.NewButtonWithIcon("Older", theme.HistoryIcon(), func() {
		a.loadOlderHistory()
	})
	a.loadOlder.Importance = widget.LowImportance
	a.loadOlder.Disable()
}

func (a *Application) buildShellButtons() shellButtons {
	buttons := shellButtons{
		refresh: widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
			a.refreshFriends()
			if a.selectedFriend != nil {
				a.loadHistory(a.selectedFriend.PublicKey)
			}
		}),
		addFriend: widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
			a.showAddFriend()
		}),
		copyKey: widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
			if a.publicKey == "" {
				return
			}
			a.window.Clipboard().SetContent(a.publicKey)
			a.setStatus("public key copied")
			a.notify("Copied", "Public key copied", widget.SuccessImportance)
		}),
		exportKey: widget.NewButtonWithIcon("Export", theme.DownloadIcon(), func() {
			a.showExportPrivateKey()
		}),
		reconnect: widget.NewButtonWithIcon("Reconnect", theme.LoginIcon(), func() {
			a.showReconnect()
		}),
		logout: widget.NewButtonWithIcon("Lock", theme.LogoutIcon(), func() {
			a.stopEvents()
			a.service.Logout()
			a.resetSessionState()
			a.showAuth()
		}),
	}
	buttons.refresh.Importance = widget.LowImportance
	buttons.addFriend.Importance = widget.LowImportance
	buttons.copyKey.Importance = widget.LowImportance
	buttons.exportKey.Importance = widget.LowImportance
	buttons.reconnect.Importance = widget.LowImportance
	buttons.logout.Importance = widget.LowImportance
	if a.offline {
		buttons.addFriend.Disable()
	}
	return buttons
}

func (a *Application) buildSidebar(buttons shellButtons) fyne.CanvasObject {
	friendTitle := widget.NewLabel("Conversations")
	friendTitle.TextStyle = fyne.TextStyle{Bold: true}
	friendStack := container.NewStack(a.friendList, container.NewCenter(a.friendEmpty))
	sidebarHeader := container.NewVBox(
		a.buildIdentityBlock(buttons),
		widget.NewSeparator(),
		container.NewGridWithColumns(3, buttons.refresh, buttons.addFriend, buttons.reconnect),
		widget.NewSeparator(),
		container.NewHBox(widget.NewIcon(theme.ListIcon()), friendTitle),
	)
	return container.NewBorder(sidebarHeader, nil, nil, nil, friendStack)
}

func (a *Application) buildIdentityBlock(buttons shellButtons) fyne.CanvasObject {
	mode := "Online"
	modeColor := successColor
	if a.offline {
		mode = "Offline"
		modeColor = warningColor
	}

	title := widget.NewLabel("Local Identity")
	title.TextStyle = fyne.TextStyle{Bold: true}
	publicKey := widget.NewLabel(shortKey(a.publicKey))
	publicKey.Importance = widget.LowImportance

	return softBlock(container.NewVBox(
		container.NewHBox(widget.NewIcon(theme.AccountIcon()), title),
		publicKey,
		container.NewHBox(newTextBadge(mode, modeColor), layout.NewSpacer()),
		container.NewGridWithColumns(2, buttons.copyKey, buttons.exportKey),
	))
}

func (a *Application) buildChatPanel(logout *widget.Button) fyne.CanvasObject {
	a.buildOutboxControls()
	composer := container.NewBorder(nil, nil, nil, a.sendButton, a.messageBox)
	messageStack := container.NewStack(a.messageList, container.NewCenter(a.messageEmpty))
	chatHeader := container.NewBorder(
		nil,
		nil,
		container.NewHBox(widget.NewIcon(theme.MailComposeIcon()), a.chatTitle, a.loadOlder),
		container.NewHBox(a.outboxLabel, a.retryOutbox, a.clearOutbox, logout),
	)
	return container.NewBorder(chatHeader, composer, nil, nil, messageStack)
}

func (a *Application) buildOutboxControls() {
	a.outboxLabel = widget.NewLabel("")
	a.outboxLabel.Importance = widget.LowImportance
	a.retryOutbox = widget.NewButtonWithIcon("Retry", theme.ViewRefreshIcon(), func() {
		a.retryFailedOutbox()
	})
	a.retryOutbox.Importance = widget.LowImportance
	if a.offline {
		a.retryOutbox.Disable()
	}
	a.clearOutbox = widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), func() {
		a.clearFailedOutbox()
	})
	a.clearOutbox.Importance = widget.LowImportance
}
