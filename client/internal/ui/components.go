package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

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

func statBlock(title string, value *widget.Label) fyne.CanvasObject {
	titleLabel := widget.NewLabel(title)
	titleLabel.Importance = widget.LowImportance
	value.TextStyle = fyne.TextStyle{Bold: true}
	value.Alignment = fyne.TextAlignCenter
	titleLabel.Alignment = fyne.TextAlignCenter
	return softBlock(container.NewVBox(value, titleLabel))
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
