package ui

import "github.com/rivo/tview"

type loader struct {
	layout  *tview.TextView
	focused bool
}

func NewLoader() *loader {

	//s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	//s.Prefix = "Fetching Data:"
	//s.Color("red")
	return &loader{
		layout:  tview.NewTextView().SetText("Fetching Data...."),
		focused: false,
	}
}
