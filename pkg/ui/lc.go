package ui

import (
	"dockyard/pkg/aws"
	"io"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/schollz/progressbar/v3"
	"github.com/sirupsen/logrus"
)

type lcFlex struct {
	layout      *tview.Flex
	tableLayout *tview.Table
	focused     bool
}

func NewLcFlex() *lcFlex {

	return &lcFlex{
		layout: tview.NewFlex(),
		tableLayout: tview.NewTable().
			SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorNavy).Attributes(tcell.AttrBold)),
		focused: false,
	}
}

// Progress bar to track rollout status
func createProgressBar(totalSize int) *progressbar.ProgressBar {
	return progressbar.NewOptions(totalSize,
		progressbar.OptionSetWriter(io.Discard),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionSetDescription("Progress "),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerPadding: " ", BarStart: "|", BarEnd: "|",
		}))
}

// Updates progress
func addProgress(bar *progressbar.ProgressBar, progress aws.RolloutProgress) {
	currentProgress := int32(progress.StepsDone)

	if currentProgress >= progress.TotalSize {
		currentProgress = int32(progress.TotalSize)
	}

	err := bar.Add(int(currentProgress))
	if err != nil {
		logrus.Error(err)
		return
	}
}

func (tui *tuiConfig) renderLcFlexWithReloading(
	asgName string,
	rolloutProgressChan aws.RolloutProgressChan,
) {
	doneChan := make(chan bool)

	tui.body.layout.SwitchToPage("4").
		SetChangedFunc(func() {
			defer close(doneChan)
			tui.body.layout.SetChangedFunc(func() {})
		})

	rolloutProgress := aws.RolloutProgress{}

	renderMutex := &sync.Mutex{}

	var bar *progressbar.ProgressBar

	tui.renderLcFlex(asgName, renderMutex, bar)

	go func() {
	loop:
		for {
			select {
			case <-doneChan:
				break loop
			case progress := <-rolloutProgressChan:
				tui.showMessage("Reloading with progress...")

				if bar == nil {
					bar = createProgressBar(int(progress.TotalSize))
				}

				addProgress(bar, progress)

				rolloutProgress.StepsSize = progress.StepsSize
				rolloutProgress.StepsDone = progress.StepsDone
				rolloutProgress.TotalSize = progress.TotalSize
				tui.renderLcFlex(asgName, renderMutex, bar)
			case <-time.After(5 * time.Second):
				tui.showMessage("Reloading...")
				tui.renderLcFlex(asgName, renderMutex, bar)
			}
		}
	}()
}

func (tui *tuiConfig) renderLcFlex(
	asgName string,
	mutex *sync.Mutex,
	progressBar *progressbar.ProgressBar,
) {
	tui.queueUpdateDraw(func() {
		mutex.Lock()
		defer mutex.Unlock()
		flexBox := tui.lcFlex.layout

		flexBox.Clear()

		asgTable := tview.NewTable().
			SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorNavy).Attributes(tcell.AttrBold))
		asgTable.SetBorders(true)

		result := [][]string{
			{"Node name", "EKS Version", "Status"},
		}

		instances, _ := tui.asgClient.GetInstanceDetailsOfAsg(asgName)

		amiIds := []*string{}
		instanceIds := make([]string, 0)
		for _, instance := range instances {
			amiIds = append(amiIds, instance.ImageId)
			instanceIds = append(instanceIds, *instance.InstanceId)
		}

		amis, _ := tui.asgClient.GetAmiDetails(amiIds)

		isNew, _ := tui.asgClient.AreInstancesNew(asgName, instanceIds)
		for i, instance := range instances {
			nodeState := "old"
			if isNew[i] {
				nodeState = "new"
			}

			var amiName *string

			for _, ami := range amis {
				if *ami.ImageId == *instance.ImageId {
					amiName = ami.Name
					break
				}
			}

			eksVersion, eksVersionErr := tui.asgClient.GetEksVersionFromAmiName(
				*amiName,
			)

			var newRow []string
			if eksVersionErr == nil {
				newRow = []string{
					*instance.PrivateDnsName,
					eksVersion,
					nodeState,
				}
			} else {
				logrus.Error(eksVersionErr)
				newRow = []string{*instance.PrivateDnsName, "", nodeState}
			}

			result = append(result, newRow)
		}

		rows, cols := len(result), len(result[0])

		for r := 0; r < rows; r++ {
			for c := 0; c < cols; c++ {
				color := tcell.ColorWhite
				notSelectable := false
				if c < 1 || r < 1 {
					color = tcell.ColorYellow
				}
				if r == 0 {
					notSelectable = true
				}
				asgTable.SetCell(r, c,
					&tview.TableCell{
						Text:          result[r][c],
						Color:         color,
						NotSelectable: notSelectable,
					},
				)

				asgTable.SetCell(r, c,
					&tview.TableCell{
						Text:          result[r][c],
						Color:         color,
						NotSelectable: notSelectable,
						Align:         tview.AlignCenter,
					},
				)
			}
		}

		lcFrame := tview.NewFrame(asgTable).
			AddText(asgName, true, tview.AlignLeft, tcell.ColorYellow)

		asgTableFlex := tview.NewFlex().
			AddItem(lcFrame, 0, 1, false).
			AddItem(tui.eventFlex.layout, 0, 1, false)
		if progressBar == nil {
			progressBar = createProgressBar(100)
		}

		if progressBar.IsFinished() {
			time.AfterFunc(3*time.Second, func() {
				tui.showInfo("Rollout done!!!")
			})
		}

		loaderFlex := tview.NewFlex().
			AddItem(nil, 0, 1, true).
			AddItem(tview.NewTextView().SetText(progressBar.String()), 0, 2, true).
			AddItem(nil, 0, 1, true)

		loaderFlex.SetBorder(true)
		asgTable.SetFixed(1, 3)
		asgTable.SetSelectable(true, false)
		flexBox.SetDirection(tview.FlexRow).
			AddItem(asgTableFlex, 0, 1, true).
			AddItem(loaderFlex, 5, 1, true)
	})
}
