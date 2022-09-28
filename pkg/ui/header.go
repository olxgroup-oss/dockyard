package ui

import (
	"log"
	"os"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type header struct {
	layout  *tview.Flex
	focused bool
}

func NewHeader() *header {

	flex := tview.NewFlex().SetDirection(tview.FlexColumn)
	return &header{
		layout:  flex,
		focused: false,
	}
}

func (tui *tuiConfig) prepareHeader() *tview.Table {
	kubectx := tui.kube.GetContext()
	k8sServerVersion, err := tui.kube.GetServerVersion()

	if err != nil {
		log.Fatal(err)
	}

	row, col := 3, 2

	headerMeta := [3][2]string{
		{"Context: ", kubectx},
		{"Server Version:", k8sServerVersion},
		{"AWS Region:", os.Getenv("AWS_REGION")},
	}
	table := tview.NewTable()

	for r := 0; r < row; r++ {
		for c := 0; c < col; c++ {
			color := tcell.ColorWhite
			if c == 0 {
				color = tcell.ColorYellow
			}
			table.SetCell(r, c,
				&tview.TableCell{
					Text:  headerMeta[r][c],
					Color: color,
					Align: tview.AlignLeft,
				},
			)
		}
	}
	return table
}
