package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

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
