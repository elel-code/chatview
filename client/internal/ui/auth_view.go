package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

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
