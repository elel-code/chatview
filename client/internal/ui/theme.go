package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

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
