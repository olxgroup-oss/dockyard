package ui

import "github.com/rivo/tview"

type TUI interface {
	RenderTUI() *tview.Flex
	setFocus(tview.Primitive)
	queueUpdateDraw(func())
	EnableEventCapture()
	toggleFocusLayout(string)
	showMessage(msg string)
	showInfo(msg string)
	showWarning(msg string)
	showError(err error)
}
