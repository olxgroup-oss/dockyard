package ui

import (
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type asgTable struct {
	layout  *tview.Table
	focused bool
}

func NewASGTable() *asgTable {
	asgTable := &asgTable{
		layout: tview.NewTable().
			SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorNavy).Attributes(tcell.AttrBold)),
		focused: false,
	}

	return asgTable
}

func (tui *tuiConfig) renderASGList(ctx context.Context) {
	tui.queueUpdateDraw(func() {

		asgtable := tui.asgTable.layout
		asgtable.Clear()
		tui.body.layout.SetBorder(true)
		clusterName := tui.kube.GetClusterName()
		asgs, err := tui.asgClient.FetchAsgOfEks(clusterName)

		if err != nil {
			tui.showError(err)
		} else {
			rows, cols := len(asgs), len(asgs[0])

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
					asgtable.SetCell(r, c,
						&tview.TableCell{
							Text:          asgs[r][c],
							Color:         color,
							NotSelectable: notSelectable,
						},
					)

					asgtable.SetCell(r, c,
						&tview.TableCell{
							Text:          asgs[r][c],
							Color:         color,
							NotSelectable: notSelectable,
							Align:         tview.AlignCenter,
						},
					)
				}
			}
			asgtable.SetFixed(1, 1)

			asgtable.SetSelectable(true, false)

			asgtable.SetDoneFunc(func(key tcell.Key) {
				if key == tcell.KeyEnter {
					fmt.Println("Selected")
				}
			}).SetSelectedFunc(func(row int, col int) {
				tui.body.layout.SwitchToPage("3")
				SetRolloutForm(ctx, tui, asgs[row][1])
			})

			tui.body.layout.SwitchToPage("1")
		}

	})
}
