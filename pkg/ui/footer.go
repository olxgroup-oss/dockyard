package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type footer struct {
	layout  *tview.TextView
	focused bool
}

func NewFooter() *footer {
	return &footer{
		layout: tview.NewTextView().
			SetTextAlign(tview.AlignCenter).
			SetTextColor(tcell.ColorGray).
			SetText(TitleFooterView),
		focused: false,
	}
}
