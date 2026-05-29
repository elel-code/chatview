package ui

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

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
