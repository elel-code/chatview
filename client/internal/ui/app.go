package ui

import (
	"context"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"chatview/internal/core"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type Application struct {
	service *core.Service
	app     fyne.App
	window  fyne.Window

	friends         []core.Friend
	messages        []core.Message
	adminUsers      []core.UserInfo
	selectedFriend  *core.Friend
	publicKey       string
	offline         bool
	historyCursor   string
	historyHasMore  bool
	historyLoading  bool
	historyRequest  int
	selectingFriend bool
	sessionVersion  int
	loginInFlight   bool
	sending         bool
	refreshing      bool
	refreshingAdmin bool
	syncingUnread   bool
	lastOutbox      core.OutboxStatus

	status       *widget.Label
	friendList   *widget.List
	friendEmpty  *emptyStateView
	messageList  *widget.List
	messageEmpty *emptyStateView
	messageBox   *widget.Entry
	sendButton   *widget.Button
	loadOlder    *widget.Button
	chatTitle    *widget.Label
	adminList    *widget.List
	adminEmpty   *emptyStateView
	outboxLabel  *widget.Label
	retryOutbox  *widget.Button
	clearOutbox  *widget.Button
	onlineCount  *widget.Label
	totalCount   *widget.Label
	bannedCount  *widget.Label

	eventCancel      context.CancelFunc
	outboxPollCancel context.CancelFunc
	sessionCtx       context.Context
	sessionCancel    context.CancelFunc
	noticePopup      *widget.PopUp
	noticeVersion    int
}

type friendListItem struct {
	*fyne.Container

	onlineDot   *canvas.Circle
	name        *widget.Label
	detail      *widget.Label
	unreadBadge *fyne.Container
	unreadText  *canvas.Text
}

type messageListItem struct {
	*fyne.Container

	leftSpacer  fyne.CanvasObject
	rightSpacer fyne.CanvasObject
	bubble      *canvas.Rectangle
	sender      *widget.Label
	body        *widget.Label
	detail      *widget.Label
}

type adminListItem struct {
	*fyne.Container

	onlineDot  *canvas.Circle
	key        *widget.Label
	stateBadge *textBadge
	action     *widget.Button
}

type textBadge struct {
	*fyne.Container

	background *canvas.Rectangle
	text       *canvas.Text
}

type emptyStateView struct {
	*fyne.Container

	label *widget.Label
}

func Run(service *core.Service) {
	fyneApp := app.NewWithID("chatview.client.fyne")
	fyneApp.Settings().SetTheme(chatTheme{base: theme.DefaultTheme()})
	window := fyneApp.NewWindow("ChatView")
	window.Resize(fyne.NewSize(1200, 800))

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapBreak
	status.Importance = widget.LowImportance

	application := &Application{
		service: service,
		app:     fyneApp,
		window:  window,
		status:  status,
	}
	application.showAuth()
	window.ShowAndRun()
}

func (a *Application) showAuth() {
	a.stopEvents()
	a.stopOutboxPoller()
	a.loginInFlight = false

	pin := widget.NewPasswordEntry()
	pin.SetPlaceHolder("PIN")

	unlock := widget.NewButtonWithIcon("Unlock", theme.LoginIcon(), func() {
		a.login(pin.Text)
	})
	unlock.Importance = widget.HighImportance
	pin.OnSubmitted = func(_ string) {
		a.login(pin.Text)
	}

	create := widget.NewButtonWithIcon("Create Identity", theme.ContentAddIcon(), func() {
		a.showCreateIdentity()
	})
	importIdentity := widget.NewButtonWithIcon("Import Identity", theme.UploadIcon(), func() {
		a.showImportIdentity()
	})

	title := widget.NewLabel("ChatView")
	title.TextStyle = fyne.TextStyle{Bold: true}
	subtitle := widget.NewLabel("Unlock local identity")
	subtitle.Importance = widget.LowImportance
	content := container.NewVBox(
		container.NewHBox(widget.NewIcon(theme.AccountIcon()), title),
		subtitle,
		pin,
		unlock,
		widget.NewSeparator(),
		container.NewGridWithColumns(2, create, importIdentity),
		a.status,
	)
	a.setContent(container.NewCenter(authPanel(content)))
}

func (a *Application) showCreateIdentity() {
	pin := widget.NewPasswordEntry()
	confirm := widget.NewPasswordEntry()
	form := dialog.NewForm("Create Identity", "Create", "Cancel", []*widget.FormItem{
		widget.NewFormItem("PIN", pin),
		widget.NewFormItem("Confirm", confirm),
	}, func(ok bool) {
		if !ok {
			return
		}
		if pin.Text == "" || pin.Text != confirm.Text {
			a.setStatus("PIN confirmation does not match")
			return
		}
		identity, err := a.service.CreateIdentity(pin.Text)
		if err != nil {
			a.setStatus(err.Error())
			return
		}
		a.setStatus("identity created")
		dialog.ShowInformation("Identity Created", "Public key:\n"+identity.PublicKey+"\n\nPrivate key:\n"+identity.PrivateKey, a.window)
	}, a.window)
	form.Resize(fyne.NewSize(720, 420))
	form.Show()
}

func (a *Application) showImportIdentity() {
	privateKey := widget.NewMultiLineEntry()
	privateKey.SetPlaceHolder("32-byte seed or 64-byte Ed25519 private key hex")
	pin := widget.NewPasswordEntry()
	form := dialog.NewForm("Import Identity", "Import", "Cancel", []*widget.FormItem{
		widget.NewFormItem("Private Key", privateKey),
		widget.NewFormItem("PIN", pin),
	}, func(ok bool) {
		if !ok {
			return
		}
		if err := a.service.ImportIdentity(strings.TrimSpace(privateKey.Text), pin.Text); err != nil {
			a.setStatus(err.Error())
			return
		}
		a.setStatus("identity imported")
	}, a.window)
	form.Resize(fyne.NewSize(720, 420))
	form.Show()
}

func (a *Application) login(pin string) {
	if a.loginInFlight {
		return
	}
	a.loginInFlight = true
	a.setStatus("logging in...")
	parent := a.sessionContext()
	go func() {
		ctx, cancel := context.WithTimeout(parent, 15*time.Second)
		defer cancel()
		result, err := a.service.Login(ctx, pin)
		fyne.Do(func() {
			a.loginInFlight = false
			if err != nil {
				state := a.service.AuthLockState()
				if state.LockedUntil != "" {
					a.setStatus(fmt.Sprintf("%s; locked until %s", err.Error(), state.LockedUntil))
				} else {
					a.setStatus(fmt.Sprintf("%s; %d attempts left", err.Error(), state.RemainingAttempts))
				}
				return
			}
			a.startSessionContext()
			a.publicKey = result.PublicKey
			a.offline = result.Offline
			if result.Offline {
				a.setStatus(fmt.Sprintf("offline unlocked as %s", shortKey(result.PublicKey)))
			} else {
				a.setStatus(fmt.Sprintf("logged in as %s", shortKey(result.PublicKey)))
			}
			a.showShell(result.Role)
			a.refreshFriends()
			if !result.Offline {
				a.service.StartOutboxWorker(a.sessionContext())
			}
			if !result.Offline && result.Role == 1 {
				a.refreshAdmin()
			}
			if !result.Offline {
				a.watchEvents()
			}
		})
	}()
}

func (a *Application) showShell(role int32) {
	a.sessionVersion++
	a.syncingUnread = false
	a.friendEmpty = emptyLabel("No conversations")
	a.messageEmpty = emptyLabel("Select a conversation")

	a.friendList = widget.NewList(
		func() int { return len(a.friends) },
		func() fyne.CanvasObject {
			return newFriendListItem()
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id < 0 || id >= len(a.friends) {
				return
			}
			row := item.(*friendListItem)
			friend := a.friends[id]
			label := friend.Alias
			if label == "" {
				label = shortKey(friend.PublicKey)
			}
			row.name.SetText(label)
			status := "offline"
			if friend.Online {
				status = "online"
			}
			row.detail.SetText(status + "  " + shortKey(friend.PublicKey))
			row.onlineDot.FillColor = offlineDotColor
			if friend.Online {
				row.onlineDot.FillColor = onlineDotColor
			}
			row.onlineDot.Refresh()
			if friend.Unread > 0 {
				row.unreadText.Text = fmt.Sprintf("%d", friend.Unread)
				row.unreadText.Refresh()
				row.unreadBadge.Show()
			} else {
				row.unreadText.Text = ""
				row.unreadText.Refresh()
				row.unreadBadge.Hide()
			}
		},
	)
	a.friendList.OnSelected = func(id widget.ListItemID) {
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

	a.messageList = widget.NewList(
		func() int { return len(a.messages) },
		func() fyne.CanvasObject {
			return newMessageListItem()
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
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
		},
	)

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
	refresh := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		a.refreshFriends()
		if a.selectedFriend != nil {
			a.loadHistory(a.selectedFriend.PublicKey)
		}
	})
	refresh.Importance = widget.LowImportance
	a.loadOlder = widget.NewButtonWithIcon("Older", theme.HistoryIcon(), func() {
		a.loadOlderHistory()
	})
	a.loadOlder.Importance = widget.LowImportance
	a.loadOlder.Disable()
	addFriend := widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
		a.showAddFriend()
	})
	addFriend.Importance = widget.LowImportance
	copyKey := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
		if a.publicKey == "" {
			return
		}
		a.window.Clipboard().SetContent(a.publicKey)
		a.setStatus("public key copied")
		a.notify("Copied", "Public key copied", widget.SuccessImportance)
	})
	copyKey.Importance = widget.LowImportance
	exportKey := widget.NewButtonWithIcon("Export", theme.DownloadIcon(), func() {
		a.showExportPrivateKey()
	})
	exportKey.Importance = widget.LowImportance
	reconnect := widget.NewButtonWithIcon("Reconnect", theme.LoginIcon(), func() {
		a.showReconnect()
	})
	reconnect.Importance = widget.LowImportance
	if a.offline {
		addFriend.Disable()
	}
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
	logout := widget.NewButtonWithIcon("Lock", theme.LogoutIcon(), func() {
		a.stopEvents()
		a.service.Logout()
		a.resetSessionState()
		a.showAuth()
	})
	logout.Importance = widget.LowImportance

	mode := "Online"
	modeColor := successColor
	if a.offline {
		mode = "Offline"
		modeColor = warningColor
	}
	modeBadge := newTextBadge(mode, modeColor)
	identityTitle := widget.NewLabel("Local Identity")
	identityTitle.TextStyle = fyne.TextStyle{Bold: true}
	publicKeyLabel := widget.NewLabel(shortKey(a.publicKey))
	publicKeyLabel.Importance = widget.LowImportance
	identityBlock := softBlock(container.NewVBox(
		container.NewHBox(widget.NewIcon(theme.AccountIcon()), identityTitle),
		publicKeyLabel,
		container.NewHBox(modeBadge, layout.NewSpacer()),
		container.NewGridWithColumns(2, copyKey, exportKey),
	))
	friendTitle := widget.NewLabel("Conversations")
	friendTitle.TextStyle = fyne.TextStyle{Bold: true}
	friendStack := container.NewStack(a.friendList, container.NewCenter(a.friendEmpty))
	sidebar := container.NewBorder(
		container.NewVBox(identityBlock, widget.NewSeparator(), container.NewGridWithColumns(3, refresh, addFriend, reconnect), widget.NewSeparator(), container.NewHBox(widget.NewIcon(theme.ListIcon()), friendTitle)),
		nil,
		nil,
		nil,
		friendStack,
	)
	composer := container.NewBorder(nil, nil, nil, a.sendButton, a.messageBox)
	messageStack := container.NewStack(a.messageList, container.NewCenter(a.messageEmpty))
	chatHeader := container.NewBorder(
		nil,
		nil,
		container.NewHBox(widget.NewIcon(theme.MailComposeIcon()), a.chatTitle, a.loadOlder),
		container.NewHBox(a.outboxLabel, a.retryOutbox, a.clearOutbox, logout),
	)
	chat := container.NewBorder(
		chatHeader,
		composer,
		nil,
		nil,
		messageStack,
	)
	chatView := container.NewHSplit(panel(sidebar), panel(chat))
	chatView.SetOffset(0.28)
	a.updateEmptyStates()
	a.updateComposerState()
	a.updateHistoryState()
	if role != 1 {
		a.setContent(container.NewBorder(nil, a.statusBar(), nil, nil, chatView))
		a.refreshOutboxStatus()
		a.startOutboxPoller()
		return
	}

	tabs := container.NewAppTabs(
		container.NewTabItem("Chat", chatView),
		container.NewTabItem("Admin", a.buildAdminView()),
	)
	a.setContent(container.NewBorder(nil, a.statusBar(), nil, nil, tabs))
	a.refreshOutboxStatus()
	a.startOutboxPoller()
}

func (a *Application) buildAdminView() fyne.CanvasObject {
	a.onlineCount = widget.NewLabel("0")
	a.totalCount = widget.NewLabel("0")
	a.bannedCount = widget.NewLabel("0")
	a.adminEmpty = emptyLabel("No users")

	stats := container.NewGridWithColumns(3,
		statBlock("Online", a.onlineCount),
		statBlock("Total", a.totalCount),
		statBlock("Banned", a.bannedCount),
	)

	refresh := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		a.refreshAdmin()
	})
	refresh.Importance = widget.LowImportance
	broadcastText := widget.NewMultiLineEntry()
	broadcastText.SetPlaceHolder("Broadcast message")
	broadcastText.Wrapping = fyne.TextWrapWord
	broadcastText.SetMinRowsVisible(3)
	broadcast := widget.NewButtonWithIcon("Broadcast", theme.MailSendIcon(), func() {
		text := strings.TrimSpace(broadcastText.Text)
		if text == "" {
			a.setStatus("broadcast text is required")
			return
		}
		if len(text) > 500 {
			a.setStatus("broadcast must be 500 characters or less")
			return
		}
		confirm := dialog.NewConfirm("Broadcast", "Send this broadcast to all users?", func(ok bool) {
			if !ok {
				return
			}
			ctx := a.sessionContext()
			go func() {
				err := a.service.Broadcast(ctx, text)
				fyne.Do(func() {
					if err != nil {
						a.setStatus(err.Error())
						a.notify("Broadcast failed", err.Error(), widget.DangerImportance)
						return
					}
					broadcastText.SetText("")
					a.setStatus("broadcast sent")
					a.notify("Broadcast sent", text, widget.SuccessImportance)
				})
			}()
		}, a.window)
		confirm.SetConfirmImportance(widget.HighImportance)
		confirm.Show()
	})
	broadcast.Importance = widget.HighImportance

	a.adminList = widget.NewList(
		func() int { return len(a.adminUsers) },
		func() fyne.CanvasObject {
			return newAdminListItem()
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id < 0 || id >= len(a.adminUsers) {
				return
			}
			row := item.(*adminListItem)
			user := a.adminUsers[id]

			row.key.SetText(user.PublicKey)
			state := "offline active"
			stateColor := mutedForegroundColor
			row.onlineDot.FillColor = offlineDotColor
			if user.Online {
				state = "online active"
				stateColor = successColor
				row.onlineDot.FillColor = onlineDotColor
			}
			if user.Banned {
				state = "banned"
				stateColor = errorColor
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
		},
	)

	title := widget.NewLabel("Administration")
	title.TextStyle = fyne.TextStyle{Bold: true}
	adminStack := container.NewStack(a.adminList, container.NewCenter(a.adminEmpty))
	top := container.NewVBox(
		container.NewBorder(nil, nil, container.NewHBox(widget.NewIcon(theme.SettingsIcon()), title), refresh),
		stats,
		widget.NewSeparator(),
		container.NewBorder(nil, nil, nil, broadcast, broadcastText),
		widget.NewSeparator(),
	)
	a.updateAdminEmpty()
	return panel(container.NewBorder(top, nil, nil, nil, adminStack))
}

func statBlock(title string, value *widget.Label) fyne.CanvasObject {
	titleLabel := widget.NewLabel(title)
	titleLabel.Importance = widget.LowImportance
	value.TextStyle = fyne.TextStyle{Bold: true}
	value.Alignment = fyne.TextAlignCenter
	titleLabel.Alignment = fyne.TextAlignCenter
	return softBlock(container.NewVBox(value, titleLabel))
}

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
			if ctx.Err() != nil || !a.isCurrentSession(version) || request != a.historyRequest || a.selectedFriend == nil || a.selectedFriend.PublicKey != publicKey {
				return
			}
			a.historyLoading = false
			if err != nil {
				a.setStatus(err.Error())
				a.updateHistoryState()
				return
			}
			a.messages = page.Messages
			a.historyCursor = page.NextCursor
			a.historyHasMore = page.HasMore
			a.messageList.Refresh()
			a.updateEmptyStates()
			a.updateHistoryState()
			if len(a.messages) > 0 {
				a.messageList.ScrollToBottom()
				last := a.messages[len(a.messages)-1]
				go func() {
					_ = a.service.MarkConversationRead(ctx, publicKey, last.ServerSeq)
				}()
			}
			a.refreshOutboxStatus()
			a.setStatus(fmt.Sprintf("%d messages", len(a.messages)))
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
			if ctx.Err() != nil || !a.isCurrentSession(version) || request != a.historyRequest || a.selectedFriend == nil || a.selectedFriend.PublicKey != peer {
				return
			}
			a.historyLoading = false
			if err != nil {
				a.setStatus(err.Error())
				a.updateHistoryState()
				return
			}
			a.messages = append(page.Messages, a.messages...)
			a.historyCursor = page.NextCursor
			a.historyHasMore = page.HasMore
			a.messageList.Refresh()
			a.updateEmptyStates()
			a.updateHistoryState()
			a.refreshOutboxStatus()
			a.setStatus(fmt.Sprintf("%d messages", len(a.messages)))
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
			a.messageList.Refresh()
			a.updateEmptyStates()
			a.messageList.ScrollToBottom()
			a.refreshOutboxStatus()
			a.refreshFriends()
			a.setStatus(message.Delivery)
		})
	}()
}

func (a *Application) refreshOutboxStatus() {
	if a.outboxLabel == nil {
		return
	}
	status := a.service.OutboxStatus()
	a.lastOutbox = status
	a.setOutboxStatus(status)
}

func (a *Application) setOutboxStatus(status core.OutboxStatus) {
	if a.outboxLabel == nil {
		return
	}
	if status.Pending == 0 && status.Failed == 0 {
		a.outboxLabel.SetText("Outbox clear")
		a.outboxLabel.Importance = widget.SuccessImportance
		if a.retryOutbox != nil {
			a.retryOutbox.Disable()
		}
		if a.clearOutbox != nil {
			a.clearOutbox.Disable()
		}
		return
	}
	a.outboxLabel.SetText(fmt.Sprintf("pending %d  failed %d", status.Pending, status.Failed))
	if status.Failed > 0 {
		a.outboxLabel.Importance = widget.WarningImportance
	} else {
		a.outboxLabel.Importance = widget.LowImportance
	}
	if a.retryOutbox != nil {
		if status.Failed > 0 && !a.offline {
			a.retryOutbox.Enable()
		} else {
			a.retryOutbox.Disable()
		}
	}
	if a.clearOutbox != nil {
		if status.Failed > 0 {
			a.clearOutbox.Enable()
		} else {
			a.clearOutbox.Disable()
		}
	}
}

func (a *Application) showAddFriend() {
	publicKey := widget.NewEntry()
	publicKey.SetPlaceHolder("Friend public key")
	dialog.ShowForm("Add Friend", "Add", "Cancel", []*widget.FormItem{
		widget.NewFormItem("Public Key", publicKey),
	}, func(ok bool) {
		if !ok {
			return
		}
		value := strings.TrimSpace(publicKey.Text)
		if value == "" {
			a.setStatus("public key is required")
			return
		}
		ctx := a.sessionContext()
		go func() {
			err := a.service.AddFriend(ctx, value)
			fyne.Do(func() {
				if ctx.Err() != nil {
					return
				}
				if err != nil {
					a.setStatus(err.Error())
					a.notify("Add friend failed", err.Error(), widget.DangerImportance)
					return
				}
				a.refreshFriends()
				a.notify("Friend added", shortKey(value), widget.SuccessImportance)
			})
		}()
	}, a.window)
}

func (a *Application) showExportPrivateKey() {
	pin := widget.NewPasswordEntry()
	privateKey := widget.NewMultiLineEntry()
	privateKey.Wrapping = fyne.TextWrapBreak
	privateKey.TextStyle = fyne.TextStyle{Monospace: true}
	privateKey.SetPlaceHolder("Private key")
	privateKey.Disable()

	reveal := widget.NewButton("Reveal", func() {
		key, err := a.service.ExportPrivateKey(pin.Text)
		if err != nil {
			a.setStatus(err.Error())
			a.notify("Export failed", err.Error(), widget.DangerImportance)
			return
		}
		privateKey.Enable()
		privateKey.SetText(key)
		privateKey.Disable()
		a.setStatus("private key revealed")
	})
	copyKey := widget.NewButton("Copy", func() {
		if privateKey.Text == "" {
			return
		}
		a.window.Clipboard().SetContent(privateKey.Text)
		a.setStatus("private key copied")
		a.notify("Copied", "Private key copied", widget.SuccessImportance)
	})

	content := container.NewBorder(
		container.NewVBox(widget.NewLabel("Enter local PIN to reveal the private key."), pin, container.NewGridWithColumns(2, reveal, copyKey)),
		nil,
		nil,
		nil,
		privateKey,
	)
	d := dialog.NewCustom("Export Private Key", "Close", content, a.window)
	d.Resize(fyne.NewSize(720, 420))
	d.Show()
}

func (a *Application) showReconnect() {
	pin := widget.NewPasswordEntry()
	form := dialog.NewForm("Reconnect", "Reconnect", "Cancel", []*widget.FormItem{
		widget.NewFormItem("PIN", pin),
	}, func(ok bool) {
		if !ok {
			return
		}
		a.login(pin.Text)
	}, a.window)
	form.Resize(fyne.NewSize(420, 180))
	form.Show()
}

func (a *Application) refreshAdmin() {
	if a.adminList == nil {
		return
	}
	if a.refreshingAdmin {
		return
	}
	a.refreshingAdmin = true
	version := a.sessionVersion
	ctx := a.sessionContext()
	a.setStatus("refreshing admin data...")
	go func() {
		update, err := a.service.PollAdminEvents(ctx)
		fyne.Do(func() {
			a.refreshingAdmin = false
			if ctx.Err() != nil || !a.isCurrentSession(version) || a.adminList == nil {
				return
			}
			if err != nil {
				a.setStatus(err.Error())
				return
			}
			a.adminUsers = update.Users
			a.onlineCount.SetText(fmt.Sprintf("%d", update.Stats.OnlineUsers))
			a.totalCount.SetText(fmt.Sprintf("%d", update.Stats.TotalUsers))
			a.bannedCount.SetText(fmt.Sprintf("%d", update.Stats.BannedUsers))
			a.adminList.Refresh()
			a.updateAdminEmpty()
			a.setStatus("admin data refreshed")
		})
	}()
}

func (a *Application) setUserBanned(publicKey string, banned bool) {
	action := "unbanning"
	if banned {
		action = "banning"
	}
	ctx := a.sessionContext()
	a.setStatus(action + " user...")
	go func() {
		err := a.service.SetUserStatus(ctx, publicKey, banned)
		fyne.Do(func() {
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				a.setStatus(err.Error())
				a.notify("Admin action failed", err.Error(), widget.DangerImportance)
				return
			}
			if banned {
				a.notify("User banned", shortKey(publicKey), widget.WarningImportance)
			} else {
				a.notify("User unbanned", shortKey(publicKey), widget.SuccessImportance)
			}
			a.refreshAdmin()
		})
	}()
}

func (a *Application) watchEvents() {
	a.stopEvents()
	ctx, cancel := context.WithCancel(a.sessionContext())
	a.eventCancel = cancel
	version := a.sessionVersion

	go func() {
		backoff := time.Second
		for {
			if ctx.Err() != nil {
				return
			}
			events, errs := a.service.Subscribe(ctx)
			var streamErr error
		stream:
			for events != nil || errs != nil {
				select {
				case event, ok := <-events:
					if !ok {
						events = nil
						continue
					}
					backoff = time.Second
					a.handleEvent(event)
				case err, ok := <-errs:
					if !ok {
						errs = nil
						continue
					}
					if err == nil {
						continue
					}
					if ctx.Err() != nil {
						return
					}
					streamErr = err
					break stream
				case <-ctx.Done():
					return
				}
			}
			if ctx.Err() != nil {
				return
			}
			fyne.Do(func() {
				if !a.isCurrentSession(version) {
					return
				}
				if streamErr != nil {
					a.setStatus("event stream reconnecting: " + streamErr.Error())
				} else {
					a.setStatus("event stream reconnecting")
				}
			})
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if backoff < 30*time.Second {
				backoff *= 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
			}
		}
	}()
}

func (a *Application) stopEvents() {
	if a.eventCancel != nil {
		a.eventCancel()
		a.eventCancel = nil
	}
}

func (a *Application) startOutboxPoller() {
	a.stopOutboxPoller()
	ctx, cancel := context.WithCancel(a.sessionContext())
	a.outboxPollCancel = cancel
	version := a.sessionVersion
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				status := a.service.OutboxStatus()
				fyne.Do(func() {
					if ctx.Err() != nil || !a.isCurrentSession(version) {
						return
					}
					previous := a.lastOutbox
					if previous == status {
						return
					}
					a.lastOutbox = status
					a.setOutboxStatus(status)
					if a.selectedFriend != nil && (status.Pending < previous.Pending || status.Failed != previous.Failed) {
						a.loadHistory(a.selectedFriend.PublicKey)
					}
				})
			}
		}
	}()
}

func (a *Application) stopOutboxPoller() {
	if a.outboxPollCancel != nil {
		a.outboxPollCancel()
		a.outboxPollCancel = nil
	}
}

func (a *Application) handleEvent(event core.Event) {
	fyne.Do(func() {
		switch event.Kind {
		case "new_message":
			body := shortKey(event.PublicKey)
			if strings.TrimSpace(event.Text) != "" {
				body += ": " + event.Text
			}
			a.notifyWithAction("New message", body, widget.HighImportance, func() {
				a.selectConversation(event.PublicKey)
			})
			a.refreshFriends()
			a.refreshOutboxStatus()
			a.syncConversation(event.PublicKey, event.Count)
		case "friend_status", "admin_update":
			a.refreshFriends()
			a.refreshAdmin()
		case "system_broadcast":
			a.notify("Broadcast", event.Text, widget.WarningImportance)
		case "force_offline":
			reason := strings.TrimSpace(event.Reason)
			if reason == "" {
				reason = "Session forced offline"
			}
			a.stopEvents()
			a.service.Logout()
			a.resetSessionState()
			a.showAuth()
			a.setStatus("force offline: " + reason)
			a.notify("Forced offline", reason, widget.DangerImportance)
		}
	})
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
			if a.selectedFriend == nil || a.selectedFriend.PublicKey != publicKey || request != a.historyRequest {
				return
			}
			if len(page.Messages) == 0 {
				return
			}
			added := a.mergeMessages(page.Messages)
			a.messageList.Refresh()
			a.updateEmptyStates()
			a.messageList.ScrollToBottom()
			a.refreshOutboxStatus()
			if len(a.messages) > 0 {
				last := a.messages[len(a.messages)-1]
				go func() {
					_ = a.service.MarkConversationRead(ctx, publicKey, last.ServerSeq)
				}()
			}
			a.setStatus(fmt.Sprintf("%d messages", len(a.messages)))
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
	peers := make([]core.Friend, 0, len(friends))
	for _, friend := range friends {
		if friend.Unread > 0 && strings.TrimSpace(friend.PublicKey) != "" {
			peers = append(peers, friend)
		}
	}
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
				if a.selectedFriend == nil || a.selectedFriend.PublicKey != peer.PublicKey || len(page.Messages) == 0 {
					return
				}
				added := a.mergeMessages(page.Messages)
				if added == 0 {
					return
				}
				a.messageList.Refresh()
				a.updateEmptyStates()
				a.messageList.ScrollToBottom()
				a.refreshOutboxStatus()
				last := a.messages[len(a.messages)-1]
				go func() {
					_ = a.service.MarkConversationRead(ctx, peer.PublicKey, last.ServerSeq)
				}()
				a.setStatus(fmt.Sprintf("%d messages", len(a.messages)))
			})
		}
	}()
}

func (a *Application) upsertMessage(message core.Message) {
	for i := range a.messages {
		if a.messages[i].ID == message.ID {
			a.messages[i] = message
			return
		}
	}
	a.messages = append(a.messages, message)
}

func (a *Application) mergeMessages(messages []core.Message) int {
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
	a.lastOutbox = core.OutboxStatus{}
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

func (a *Application) retryFailedOutbox() {
	if a.offline {
		a.setStatus("offline")
		return
	}
	if a.retryOutbox != nil {
		a.retryOutbox.Disable()
	}
	a.setStatus("retrying failed messages...")
	go func() {
		err := a.service.RetryFailedOutbox()
		fyne.Do(func() {
			if err != nil {
				a.setStatus(err.Error())
				a.notify("Retry failed", err.Error(), widget.DangerImportance)
				a.refreshOutboxStatus()
				return
			}
			a.refreshOutboxStatus()
			if a.selectedFriend != nil {
				a.loadHistory(a.selectedFriend.PublicKey)
			}
			a.setStatus("failed messages queued")
			a.notify("Retry queued", "Failed messages will be sent again", widget.SuccessImportance)
		})
	}()
}

func (a *Application) clearFailedOutbox() {
	confirm := dialog.NewConfirm("Clear Failed", "Remove failed outbox messages from the local cache?", func(ok bool) {
		if !ok {
			return
		}
		if a.clearOutbox != nil {
			a.clearOutbox.Disable()
		}
		a.setStatus("clearing failed messages...")
		go func() {
			err := a.service.ClearFailedOutbox()
			fyne.Do(func() {
				if err != nil {
					a.setStatus(err.Error())
					a.notify("Clear failed", err.Error(), widget.DangerImportance)
					a.refreshOutboxStatus()
					return
				}
				a.refreshOutboxStatus()
				if a.selectedFriend != nil {
					a.loadHistory(a.selectedFriend.PublicKey)
				}
				a.setStatus("failed messages cleared")
				a.notify("Outbox cleared", "Failed messages removed", widget.SuccessImportance)
			})
		}()
	}, a.window)
	confirm.SetConfirmImportance(widget.DangerImportance)
	confirm.Show()
}

func (a *Application) confirmSetUserBanned(publicKey string, banned bool) {
	title := "Ban User"
	message := "Ban " + shortKey(publicKey) + "?"
	importance := widget.DangerImportance
	if !banned {
		title = "Unban User"
		message = "Unban " + shortKey(publicKey) + "?"
		importance = widget.SuccessImportance
	}
	confirm := dialog.NewConfirm(title, message, func(ok bool) {
		if ok {
			a.setUserBanned(publicKey, banned)
		}
	}, a.window)
	confirm.SetConfirmImportance(importance)
	confirm.Show()
}

func (a *Application) notify(title, body string, importance widget.Importance) {
	a.notifyWithAction(title, body, importance, nil)
}

func (a *Application) notifyWithAction(title, body string, importance widget.Importance, action func()) {
	body = strings.TrimSpace(body)
	if body == "" {
		body = title
	}
	if a.app != nil {
		a.app.SendNotification(fyne.NewNotification(title, truncateText(body, 180)))
	}
	a.noticeVersion++
	version := a.noticeVersion
	if a.noticePopup != nil {
		a.noticePopup.Hide()
		a.noticePopup = nil
	}

	titleLabel := widget.NewLabel(title)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	titleLabel.Importance = importance
	bodyLabel := widget.NewLabel(truncateText(body, 240))
	bodyLabel.Wrapping = fyne.TextWrapWord
	bodyLabel.Importance = widget.LowImportance
	bodyContent := fyne.CanvasObject(bodyLabel)
	if action != nil {
		openButton := widget.NewButtonWithIcon("Open", theme.NavigateNextIcon(), func() {
			if a.noticePopup != nil {
				a.noticePopup.Hide()
				a.noticePopup = nil
			}
			action()
		})
		openButton.Importance = widget.HighImportance
		bodyContent = container.NewBorder(nil, nil, nil, openButton, bodyLabel)
	}
	closeButton := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		if a.noticePopup != nil {
			a.noticePopup.Hide()
			a.noticePopup = nil
		}
	})
	closeButton.Importance = widget.LowImportance
	content := panel(container.NewBorder(
		container.NewBorder(nil, nil, widget.NewIcon(iconForImportance(importance)), closeButton, titleLabel),
		nil,
		nil,
		nil,
		bodyContent,
	))
	popup := widget.NewPopUp(content, a.window.Canvas())
	size := fyne.NewSize(360, content.MinSize().Height+theme.Padding()*2)
	pos := fyne.NewPos(a.window.Canvas().Size().Width-size.Width-theme.Padding()*2, theme.Padding()*2)
	if pos.X < theme.Padding() {
		pos.X = theme.Padding()
	}
	popup.Resize(size)
	popup.ShowAtPosition(pos)
	a.noticePopup = popup

	go func() {
		time.Sleep(4 * time.Second)
		fyne.Do(func() {
			if a.noticeVersion == version && a.noticePopup == popup {
				popup.Hide()
				a.noticePopup = nil
			}
		})
	}()
}

func (a *Application) setContent(content fyne.CanvasObject) {
	a.window.SetContent(appFrame(content))
}

func appFrame(content fyne.CanvasObject) fyne.CanvasObject {
	return container.NewStack(appBackground(), container.NewPadded(content))
}

func panel(content fyne.CanvasObject) fyne.CanvasObject {
	background := canvas.NewRectangle(panelBackgroundColor)
	background.StrokeColor = separatorColor
	background.StrokeWidth = 1
	background.CornerRadius = 8
	return container.NewStack(background, container.NewPadded(content))
}

func authPanel(content fyne.CanvasObject) fyne.CanvasObject {
	background := canvas.NewRectangle(authPanelBackgroundColor)
	background.StrokeColor = separatorColor
	background.StrokeWidth = 1
	background.CornerRadius = 8
	return container.NewStack(background, container.NewPadded(content))
}

func appBackground() fyne.CanvasObject {
	return canvas.NewRectangle(appBackgroundColor)
}

func newFriendListItem() fyne.CanvasObject {
	onlineDot := canvas.NewCircle(offlineDotColor)
	dot := container.NewGridWrap(fyne.NewSize(10, 10), onlineDot)

	name := widget.NewLabel("")
	name.TextStyle = fyne.TextStyle{Bold: true}
	name.Truncation = fyne.TextTruncateEllipsis
	detail := widget.NewLabel("")
	detail.Importance = widget.LowImportance
	detail.Truncation = fyne.TextTruncateEllipsis

	unreadText := canvas.NewText("", foregroundOnAccentColor)
	unreadText.Alignment = fyne.TextAlignCenter
	unreadText.TextSize = theme.TextSize() - 2
	unreadText.TextStyle = fyne.TextStyle{Bold: true}
	unreadBackground := canvas.NewRectangle(unreadBadgeColor)
	unreadBackground.CornerRadius = 10
	unreadBadge := container.NewGridWrap(
		fyne.NewSize(34, 22),
		container.NewStack(unreadBackground, container.NewCenter(unreadText)),
	)
	unreadBadge.Hide()

	body := container.NewBorder(
		nil,
		nil,
		container.NewCenter(dot),
		unreadBadge,
		container.NewVBox(name, detail),
	)
	row := container.NewPadded(body)
	return &friendListItem{
		Container:   row,
		onlineDot:   onlineDot,
		name:        name,
		detail:      detail,
		unreadBadge: unreadBadge,
		unreadText:  unreadText,
	}
}

func newMessageListItem() fyne.CanvasObject {
	sender := widget.NewLabel("")
	sender.TextStyle = fyne.TextStyle{Bold: true}
	body := widget.NewLabel("")
	body.Wrapping = fyne.TextWrapWord
	detail := widget.NewLabel("")
	detail.Importance = widget.LowImportance

	bubble := canvas.NewRectangle(incomingBubbleColor)
	bubble.StrokeColor = bubbleBorderColor
	bubble.StrokeWidth = 1
	bubble.CornerRadius = 8
	bubbleBox := container.NewStack(bubble, container.NewPadded(container.NewVBox(sender, body, detail)))
	leftSpacer := layout.NewSpacer()
	rightSpacer := layout.NewSpacer()
	leftSpacer.Hide()

	row := container.NewHBox(leftSpacer, bubbleBox, rightSpacer)
	return &messageListItem{
		Container:   row,
		leftSpacer:  leftSpacer,
		rightSpacer: rightSpacer,
		bubble:      bubble,
		sender:      sender,
		body:        body,
		detail:      detail,
	}
}

func newAdminListItem() fyne.CanvasObject {
	onlineDot := canvas.NewCircle(offlineDotColor)
	dot := container.NewGridWrap(fyne.NewSize(10, 10), onlineDot)
	key := widget.NewLabel("")
	key.Truncation = fyne.TextTruncateEllipsis
	key.TextStyle = fyne.TextStyle{Bold: true}
	stateBadge := newTextBadge("offline active", mutedForegroundColor)
	action := widget.NewButtonWithIcon("Ban", theme.CancelIcon(), nil)
	action.Importance = widget.DangerImportance

	meta := container.NewHBox(stateBadge, action)
	row := container.NewPadded(container.NewBorder(
		nil,
		nil,
		container.NewCenter(dot),
		meta,
		key,
	))
	return &adminListItem{
		Container:  row,
		onlineDot:  onlineDot,
		key:        key,
		stateBadge: stateBadge,
		action:     action,
	}
}

func softBlock(content fyne.CanvasObject) fyne.CanvasObject {
	background := canvas.NewRectangle(softBlockBackgroundColor)
	background.StrokeColor = separatorColor
	background.StrokeWidth = 1
	background.CornerRadius = 8
	return container.NewStack(background, container.NewPadded(content))
}

func newTextBadge(text string, fill color.Color) *textBadge {
	label := canvas.NewText(text, foregroundOnAccentColor)
	label.Alignment = fyne.TextAlignCenter
	label.TextSize = theme.TextSize() - 2
	label.TextStyle = fyne.TextStyle{Bold: true}
	background := canvas.NewRectangle(fill)
	background.CornerRadius = 10
	content := container.NewGridWrap(
		fyne.NewSize(104, 22),
		container.NewStack(background, container.NewCenter(label)),
	)
	return &textBadge{
		Container:  content,
		background: background,
		text:       label,
	}
}

func setTextBadge(badge *textBadge, text string, fill color.Color) {
	badge.text.Text = text
	badge.background.FillColor = fill
	badge.text.Refresh()
	badge.background.Refresh()
	badge.Container.Refresh()
}

type chatTheme struct {
	base fyne.Theme
}

var (
	appBackgroundColor       = color.NRGBA{R: 0xe7, G: 0xee, B: 0xeb, A: 0xff}
	panelBackgroundColor     = color.NRGBA{R: 0xee, G: 0xf3, B: 0xf1, A: 0xff}
	authPanelBackgroundColor = color.NRGBA{R: 0xf0, G: 0xf4, B: 0xf2, A: 0xff}
	overlayBackgroundColor   = color.NRGBA{R: 0xf5, G: 0xf8, B: 0xf7, A: 0xff}
	menuBackgroundColor      = color.NRGBA{R: 0xf1, G: 0xf5, B: 0xf3, A: 0xff}
	inputBackgroundColor     = color.NRGBA{R: 0xed, G: 0xf2, B: 0xf3, A: 0xff}
	buttonBackgroundColor    = color.NRGBA{R: 0xe9, G: 0xf0, B: 0xed, A: 0xff}
	disabledButtonColor      = color.NRGBA{R: 0xd9, G: 0xe2, B: 0xde, A: 0xff}
	hoverColor               = color.NRGBA{R: 0x07, G: 0x22, B: 0x1d, A: 0x0f}
	pressedColor             = color.NRGBA{R: 0x07, G: 0x22, B: 0x1d, A: 0x42}
	shadowColor              = color.NRGBA{R: 0x07, G: 0x22, B: 0x1d, A: 0x0c}
	scrollBarColor           = color.NRGBA{R: 0x35, G: 0x4a, B: 0x4f, A: 0x58}
	scrollBarBackgroundColor = color.NRGBA{R: 0x07, G: 0x22, B: 0x1d, A: 0x08}
	separatorColor           = color.NRGBA{R: 0xbd, G: 0xca, B: 0xd3, A: 0xff}
	foregroundColor          = color.NRGBA{R: 0x18, G: 0x20, B: 0x2f, A: 0xff}
	mutedForegroundColor     = color.NRGBA{R: 0x5b, G: 0x68, B: 0x73, A: 0xff}
	placeholderColor         = color.NRGBA{R: 0x66, G: 0x75, B: 0x7d, A: 0xff}
	primaryColor             = color.NRGBA{R: 0x1b, G: 0x76, B: 0x62, A: 0xff}
	successColor             = color.NRGBA{R: 0x1f, G: 0x7a, B: 0x4f, A: 0xff}
	warningColor             = color.NRGBA{R: 0xa3, G: 0x62, B: 0x12, A: 0xff}
	errorColor               = color.NRGBA{R: 0xb4, G: 0x2d, B: 0x3b, A: 0xff}
	foregroundOnAccentColor  = color.NRGBA{R: 0xf4, G: 0xfb, B: 0xf8, A: 0xff}
	focusColor               = color.NRGBA{R: 0x07, G: 0x22, B: 0x1d, A: 0x22}
	selectionColor           = color.NRGBA{R: 0x1b, G: 0x76, B: 0x62, A: 0x34}
	softBlockBackgroundColor = color.NRGBA{R: 0xe5, G: 0xed, B: 0xea, A: 0xff}
	incomingBubbleColor      = color.NRGBA{R: 0xf3, G: 0xf6, B: 0xf7, A: 0xff}
	outgoingBubbleColor      = color.NRGBA{R: 0xd8, G: 0xec, B: 0xe5, A: 0xff}
	bubbleBorderColor        = color.NRGBA{R: 0xcc, G: 0xd8, B: 0xdd, A: 0xff}
	unreadBadgeColor         = color.NRGBA{R: 0x1b, G: 0x76, B: 0x62, A: 0xff}
	onlineDotColor           = color.NRGBA{R: 0x1f, G: 0x7a, B: 0x4f, A: 0xff}
	offlineDotColor          = color.NRGBA{R: 0x94, G: 0xa1, B: 0xaa, A: 0xff}
)

func (t chatTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return appBackgroundColor
	case theme.ColorNameOverlayBackground:
		return overlayBackgroundColor
	case theme.ColorNameMenuBackground:
		return menuBackgroundColor
	case theme.ColorNameInputBackground:
		return inputBackgroundColor
	case theme.ColorNameForeground:
		return foregroundColor
	case theme.ColorNameButton:
		return buttonBackgroundColor
	case theme.ColorNameDisabledButton:
		return disabledButtonColor
	case theme.ColorNameDisabled:
		return mutedForegroundColor
	case theme.ColorNamePlaceHolder:
		return placeholderColor
	case theme.ColorNameHover:
		return hoverColor
	case theme.ColorNamePressed:
		return pressedColor
	case theme.ColorNameShadow:
		return shadowColor
	case theme.ColorNameScrollBar:
		return scrollBarColor
	case theme.ColorNameScrollBarBackground:
		return scrollBarBackgroundColor
	case theme.ColorNameSeparator, theme.ColorNameInputBorder:
		return separatorColor
	case theme.ColorNamePrimary, theme.ColorNameHyperlink:
		return primaryColor
	case theme.ColorNameSuccess:
		return successColor
	case theme.ColorNameWarning:
		return warningColor
	case theme.ColorNameError:
		return errorColor
	case theme.ColorNameForegroundOnPrimary, theme.ColorNameForegroundOnSuccess, theme.ColorNameForegroundOnWarning, theme.ColorNameForegroundOnError:
		return foregroundOnAccentColor
	case theme.ColorNameFocus:
		return focusColor
	case theme.ColorNameSelection:
		return selectionColor
	}
	return t.base.Color(name, theme.VariantLight)
}

func (t chatTheme) Font(style fyne.TextStyle) fyne.Resource {
	return t.base.Font(style)
}

func (t chatTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}

func (t chatTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameScrollBar:
		return 8
	case theme.SizeNameScrollBarSmall:
		return 5
	case theme.SizeNameScrollBarRadius:
		return 4
	}
	return t.base.Size(name)
}

func emptyLabel(text string) *emptyStateView {
	label := widget.NewLabel(text)
	label.Alignment = fyne.TextAlignCenter
	label.Importance = widget.LowImportance
	content := softBlock(container.NewHBox(layout.NewSpacer(), label, layout.NewSpacer()))
	return &emptyStateView{
		Container: content.(*fyne.Container),
		label:     label,
	}
}

func (v *emptyStateView) SetText(text string) {
	if v == nil || v.label == nil {
		return
	}
	v.label.SetText(text)
}

func messageDetail(message core.Message) string {
	parts := make([]string, 0, 3)
	if message.Timestamp != "" {
		parts = append(parts, message.Timestamp)
	}
	if message.Delivery != "" {
		parts = append(parts, message.Delivery)
	}
	if message.Error != "" {
		parts = append(parts, message.Error)
	}
	return strings.Join(parts, "  ")
}

func sortMessages(messages []core.Message) {
	sort.SliceStable(messages, func(i, j int) bool {
		left := messages[i]
		right := messages[j]
		if left.ServerSeq > 0 && right.ServerSeq > 0 && left.ServerSeq != right.ServerSeq {
			return left.ServerSeq < right.ServerSeq
		}
		if left.Timestamp != "" && right.Timestamp != "" && left.Timestamp != right.Timestamp {
			return left.Timestamp < right.Timestamp
		}
		if left.ServerSeq != right.ServerSeq {
			return left.ServerSeq < right.ServerSeq
		}
		return left.ID < right.ID
	})
}

func iconForImportance(importance widget.Importance) fyne.Resource {
	switch importance {
	case widget.DangerImportance:
		return theme.ErrorIcon()
	case widget.WarningImportance:
		return theme.WarningIcon()
	case widget.SuccessImportance:
		return theme.ConfirmIcon()
	default:
		return theme.InfoIcon()
	}
}

func truncateText(text string, limit int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

func shortKey(publicKey string) string {
	if len(publicKey) <= 12 {
		return publicKey
	}
	return publicKey[:8] + "..." + publicKey[len(publicKey)-4:]
}
