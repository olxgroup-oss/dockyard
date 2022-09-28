package ui

import "github.com/rivo/tview"

type body struct {
	layout  *tview.Pages
	focused bool
}

func NewBody() *body {

	return &body{
		layout:  NewPages(),
		focused: false,
	}
}

func NewPages() *tview.Pages {
	return tview.NewPages()
}
