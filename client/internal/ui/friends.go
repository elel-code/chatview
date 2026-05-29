package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

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
