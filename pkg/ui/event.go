package ui

import (
	"bytes"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type eventFlex struct {
	layout          *tview.Flex
	eventTextLayout *tview.TextView
	eventTextFrame  *tview.Frame
	events          chan string
	focused         bool
}

func NewEventFlex() *eventFlex {

	eventTextLayout := tview.NewTextView()

	eventTextFrame := tview.NewFrame(eventTextLayout).
		AddText("Rollout Events", true, tview.AlignLeft, tcell.ColorYellow)

	//eventTextFrame.SetBorder(true)
	layout := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(eventTextFrame, 0, 1, false)

	eF := &eventFlex{
		layout:          layout,
		eventTextLayout: eventTextLayout,
		eventTextFrame:  eventTextFrame,
		focused:         false,
		events:          make(chan string),
	}

	go func(e *eventFlex) {
		e.UpdateText()
	}(eF)
	return eF
}

// Parse context
func (e *eventFlex) UpdateText() {
	var buffer bytes.Buffer

	for event := range e.events {
		buffer.WriteString(event)
		buffer.WriteString("\n")
		e.eventTextLayout.SetText(buffer.String())
	}
}
