package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

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
		a.confirmBroadcast(broadcastText)
	})
	broadcast.Importance = widget.HighImportance

	a.adminList = widget.NewList(
		func() int { return len(a.adminUsers) },
		func() fyne.CanvasObject {
			return newAdminListItem()
		},
		a.bindAdminListItem,
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

func (a *Application) confirmBroadcast(input *widget.Entry) {
	text := strings.TrimSpace(input.Text)
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
				input.SetText("")
				a.setStatus("broadcast sent")
				a.notify("Broadcast sent", text, widget.SuccessImportance)
			})
		}()
	}, a.window)
	confirm.SetConfirmImportance(widget.HighImportance)
	confirm.Show()
}
