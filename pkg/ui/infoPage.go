package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type infoPage struct {
	layout  *tview.Flex
	focused bool
}

func NewInfoPage() *infoPage {

	return &infoPage{
		layout:  tview.NewFlex(),
		focused: false,
	}
}

// Info Modal
func SetInfoLayout(
	flexBox *tview.Flex,
	message string,
	infoType string,
	doneFunc func(),
) {

	flexBox.Clear()

	saveButton := tview.NewButton("ok")
	saveButton.SetBackgroundColor(tcell.ColorGreen)

	saveButtonFlex := tview.NewFlex().
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorNavajoWhite), 15, 1, true).
		AddItem(saveButton, 0, 1, true).
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorNavajoWhite), 15, 1, true)

	saveButton.SetFocusFunc(doneFunc)

	modalFlex := tview.NewFlex()

	messageView := tview.NewTextView().
		SetText(message).
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorBlack)
	messageView.SetBackgroundColor(tcell.ColorNavajoWhite)

	modalFlex.SetDirection(tview.FlexRow).
		AddItem(messageView, 0, 1, false).
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorNavajoWhite), 0, 1, false).
		AddItem(saveButtonFlex, 1, 1, true)

	modalFlex.SetBackgroundColor(tcell.ColorNavajoWhite)
	modalFlex.SetBorder(true).
		SetBorderColor(tcell.ColorGreen).
		SetBorderAttributes(tcell.AttrDim).
		SetTitle("Info").
		SetTitleColor(tcell.ColorGreen).
		SetTitleAlign(tview.AlignCenter)

	flexBox.
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(modalFlex, 20, 1, true).
			AddItem(nil, 0, 1, false), 0, 1, true).
		AddItem(nil, 0, 1, false)

}
