package ui

import (
	"context"

	"chatview/client/internal/core"
	"chatview/client/internal/domain"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type Application struct {
	service *core.Service
	app     fyne.App
	window  fyne.Window

	friends         []domain.Friend
	messages        []domain.Message
	adminUsers      []domain.UserInfo
	selectedFriend  *domain.Friend
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
	lastOutbox      domain.OutboxStatus

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
