package ui

import (
	"context"
	"dockyard/pkg/aws"
	"time"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type newRollout struct {
	layout  *tview.Flex
	focused bool
}

func NewRollout() *newRollout {

	return &newRollout{
		layout:  tview.NewFlex(),
		focused: false,
	}
}

func SetRolloutForm(ctx context.Context, tui *tuiConfig, asgName string) {
	minNodes, _ := tui.asgClient.GetTagValueOfAsg(asgName, "dockyard.io/min")

	maxNodes, _ := tui.asgClient.GetTagValueOfAsg(asgName, "dockyard.io/max")

	hasRolloutStarted := false

	if minNodes != 0 || maxNodes != 0 {
		hasRolloutStarted = true
	}

	flexBox := tui.rolloutForm.layout
	flexBox.Clear()
	// Temp constant batch size
	batchSizeTextView := tview.NewTextView().
		SetText("Batch Size: 1").
		SetTextColor(tcell.ColorBlack)
	batchSizeTextView.SetBackgroundColor(tcell.ColorBlue)

	batchSizeInputView := tview.NewInputField().
		SetFieldTextColor(tcell.ColorBlack).
		SetFieldBackgroundColor(tcell.ColorWhite).
		SetAcceptanceFunc(func(input string, lastChar rune) bool {
			return unicode.IsNumber(lastChar)
		})
	batchSizeInputView.SetBackgroundColor(tcell.ColorBlue)
	batchFormFlex := tview.NewFlex().
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorBlue), 10, 1, true).
		AddItem(batchSizeTextView, 0, 1, true).
		//AddItem(batchSizeInputView, 0, 1, true).
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorBlue), 10, 1, true)

	batchSizeWarningText := tview.NewTextView().
		SetText("Batch Size should be less than the max nodes").
		SetTextColor(tcell.ColorAntiqueWhite).
		SetWrap(true)
	batchSizeWarningText.SetBackgroundColor(tcell.ColorBlue)
	batchSizeWarningFlex := tview.NewFlex().
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorBlue), 5, 1, false).
		AddItem(batchSizeWarningText, 0, 3, true).
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorBlue), 3, 1, false)

	rolloutContinueText := tview.NewTextView().
		SetText("Rollout has already started!!").
		SetTextColor(tcell.ColorDarkRed).
		SetWrap(true)
	rolloutContinueText.SetBackgroundColor(tcell.ColorBlue)
	rolloutContinueFlex := tview.NewFlex().
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorBlue), 5, 1, false).
		AddItem(rolloutContinueText, 0, 3, true).
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorBlue), 3, 1, false)

	var buttonText string

	if hasRolloutStarted {
		buttonText = "Continue Rollout"
	} else {
		buttonText = "Start Rollout"
	}

	saveButton := tview.NewButton(buttonText)
	saveButton.SetBackgroundColor(tcell.ColorGreen)
	saveButton.SetSelectedFunc(func() {
		progressChan := make(aws.RolloutProgressChan)
		tui.renderLcFlexWithReloading(asgName, progressChan)

		go func() {
			rolloutSuccess := false
			tui.sidebar.layout.DisableSelection()
			err := tui.asgClient.StartRollout(
				ctx,
				asgName,
				// TODO Make this configurable
				1,
				progressChan,
				tui.eventFlex.events,
			)
			if err != nil {
				tui.showError(err)
				tui.sidebar.layout.EnableSelection()
			} else {
				rolloutSuccess = true
			}
			time.Sleep(time.Duration(tui.asgRolloutConfig.PeriodWait.BeforePost) * time.Second)
			err = tui.asgClient.PostRolloutStart(
				asgName,
				progressChan,
				tui.eventFlex.events,
				rolloutSuccess,
			)
			if err != nil {
				tui.showError(err)
				tui.sidebar.layout.EnableSelection()
			}
			tui.sidebar.layout.EnableSelection()
		}()

	})

	saveButtonFlex := tview.NewFlex().
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorBlue), 15, 1, true).
		AddItem(saveButton, 0, 1, true).
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorBlue), 15, 1, true)

	formFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorBlue), 2, 1, true).
		AddItem(batchFormFlex, 2, 1, true).
		AddItem(batchSizeWarningFlex, 0, 1, true)

	if hasRolloutStarted {
		formFlex.AddItem(rolloutContinueFlex, 0, 1, true)
	}

	formFlex.
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorBlue), 5, 1, true).
		AddItem(saveButtonFlex, 1, 1, true).
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorBlue), 0, 1, true)

	formFlex.SetBorder(true)
	formFlex.SetBorderColor(tcell.ColorGreen)
	formFlex.SetTitle(asgName)

	flexBox.
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(formFlex, 0, 1, true).
			AddItem(nil, 0, 1, false), 0, 1, true).
		AddItem(nil, 0, 1, false)
}
