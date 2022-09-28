package ui

import (
	"context"
	"dockyard/pkg/aws"
	"dockyard/pkg/kube"
	"sync"
	"time"

	"github.com/common-nighthawk/go-figure"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var (
	TitleFooterView = "Navigate using Ctrl+h Ctrl+l"
	TitleHeaderView = figure.NewFigure("DOCKYARD", "basic", true).String()
)

// dockyard custom tview components
type tuiLayout struct {
	header            *header
	footer            *footer
	body              *body
	sidebar           *sidebar
	loader            *loader
	asgTable          *asgTable
	preflightFlex     *preflight
	lcFlex            *lcFlex
	infoPage          *infoPage
	rolloutForm       *newRollout
	eventFlex         *eventFlex
	messageModalMutex *sync.Mutex
	//rolloutTimeouts   *RolloutTimeouts
}

type tuiConfig struct {
	tuiLayout
	App              *tview.Application
	kube             kube.KubeClient
	asgClient        aws.AsgRolloutClient
	awsEksClient     aws.AwsEksClient
	awsConfig        *aws.AwsConfig
	asgRolloutConfig *aws.AsgRolloutConfig
}

// Initialize dockyard tview components
func NewTUI(
	ctx context.Context,
	kubeClient kube.KubeClient,
	awsConfig *aws.AwsConfig,
	asgRolloutConfig *aws.AsgRolloutConfig,
) *tuiConfig {

	// UI initialize
	tui := &tuiConfig{
		awsConfig: awsConfig,
		tuiLayout: tuiLayout{
			header:            NewHeader(),
			footer:            NewFooter(),
			body:              NewBody(),
			sidebar:           NewSidebar(),
			loader:            NewLoader(),
			asgTable:          NewASGTable(),
			preflightFlex:     NewPreflight(),
			lcFlex:            NewLcFlex(),
			infoPage:          NewInfoPage(),
			rolloutForm:       NewRollout(),
			eventFlex:         NewEventFlex(),
			messageModalMutex: &sync.Mutex{},
		},
		App:       tview.NewApplication(),
		kube:      kubeClient,
		asgClient: aws.NewAsgRollout(ctx, awsConfig, kubeClient, asgRolloutConfig),
		awsEksClient: aws.NewAwsEKS(
			kubeClient.GetClusterName(),
			awsConfig.GetProfile(),
		),
		asgRolloutConfig: asgRolloutConfig,
	}

	tui.body.layout.AddPage("-1", tui.infoPage.layout, true, false)
	tui.body.layout.AddPage("0", tui.loader.layout, true, false)
	tui.body.layout.AddPage("1", tui.asgTable.layout, true, false)
	tui.body.layout.AddPage("2", tui.preflightFlex.layout, true, false)
	tui.body.layout.AddPage("3", tui.rolloutForm.layout, true, false)
	tui.body.layout.AddPage("4", tui.lcFlex.layout, true, false)

	tui.asgTable.layout.SetBorders(true).SetTitle("Node Groups").SetBorder(true)
	tui.body.layout.SetBorder(true)
	tui.footer.layout.SetBorder(true).SetTitle("Help")
	tui.header.layout.SetBorder(true)
	tui.sidebar.layout.asgRolloutTree.SetBorder(true).SetTitle("Options")
	tui.header.layout.
		AddItem(tui.prepareHeader(), 0, 2, false).
		AddItem(tview.NewTextView().SetTextAlign(tview.AlignRight).SetTextColor(tcell.ColorYellowGreen).SetText(TitleHeaderView), 0, 2, false)

	// Handlers
	tui.sidebar.layout.asgRolloutTree.SetSelectedFunc(
		func(node *tview.TreeNode) {
			reference := node.GetReference()
			if reference == nil {
				return // Selecting the root node does nothing.
			} else if reference == "ASG Rollouts" {
				tui.body.layout.SwitchToPage("0")
				tui.renderASGList(ctx)
			} else if reference == "Preflight checks" {
				tui.body.layout.SwitchToPage("0")
				tui.renderPreflightFlex()
			}
		},
	)

	return tui
}

// Create a flex wrapper for all child components
func (tui *tuiConfig) RenderTUI() *tview.Flex {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(tui.header.layout, 0, 2, false), 0, 2, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(tui.sidebar.layout.asgRolloutTree, 0, 1, false).
			AddItem(tui.body.layout, 0, 7, false), 0, 8, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(tui.footer.layout, 0, 1, false), 0, 1, false)
}

func (tui *tuiConfig) setFocus(p tview.Primitive) {
	tui.queueUpdateDraw(func() {
		tui.App.SetFocus(p)
	})
}

func (tui *tuiConfig) queueUpdateDraw(f func()) {
	go func() {
		tui.App.QueueUpdateDraw(f)
	}()
}

// Enable keyboard event capture for navigation
func (tui *tuiConfig) EnableEventCapture() {

	type (
		KeyOp int16
	)

	const (
		KeyTop KeyOp = iota
		KeySidebar
		KeyMain
		KeyBottom
	)

	var (
		KeyMapping = map[KeyOp]tcell.Key{
			KeyTop:     tcell.KeyCtrlK,
			KeySidebar: tcell.KeyCtrlH,
			KeyBottom:  tcell.KeyCtrlJ,
			KeyMain:    tcell.KeyCtrlL,
		}
	)
	tui.App.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case KeyMapping[KeyTop]:
		case KeyMapping[KeyBottom]:
		case KeyMapping[KeySidebar]:
			tui.setFocus(tui.sidebar.layout.asgRolloutTree)
			tui.showMessage("Navigate j/k for up/down")
			tui.toggleFocusLayout("sidebar")
		case KeyMapping[KeyMain]:
			tui.setFocus(tui.body.layout)
			tui.showMessage("Navigate j/k for up/down")
			tui.toggleFocusLayout("body")
		}
		return event
	})
}

// Highlights selected layout
func (tui *tuiConfig) toggleFocusLayout(selectedLayout string) {

	switch selectedLayout {
	case "header":
		tui.header.layout.SetBorderColor(tcell.ColorYellow)
		tui.footer.layout.SetBorderColor(tcell.ColorWhite)
		tui.body.layout.SetBorderColor(tcell.ColorWhite)
		tui.sidebar.layout.asgRolloutTree.SetBorderColor(tcell.ColorWhite)
	case "footer":
		tui.header.layout.SetBorderColor(tcell.ColorWhite)
		tui.footer.layout.SetBorderColor(tcell.ColorYellow)
		tui.body.layout.SetBorderColor(tcell.ColorWhite)
		tui.sidebar.layout.asgRolloutTree.SetBorderColor(tcell.ColorWhite)
	case "sidebar":
		tui.header.layout.SetBorderColor(tcell.ColorWhite)
		tui.footer.layout.SetBorderColor(tcell.ColorWhite)
		tui.body.layout.SetBorderColor(tcell.ColorWhite)
		tui.sidebar.layout.asgRolloutTree.SetBorderColor(tcell.ColorYellow)
	case "body":
		tui.header.layout.SetBorderColor(tcell.ColorWhite)
		tui.footer.layout.SetBorderColor(tcell.ColorWhite)
		tui.body.layout.SetBorderColor(tcell.ColorYellow)
		tui.sidebar.layout.asgRolloutTree.SetBorderColor(tcell.ColorWhite)
	}

}
func (tui *tuiConfig) resetMessage() {
	tui.queueUpdateDraw(func() {
		tui.footer.layout.SetText(TitleFooterView).SetTextColor(tcell.ColorGray)
	})
}

func (tui *tuiConfig) showMessageModal(msg, infoType string) {

	tui.messageModalMutex.Lock()

	name, _ := tui.body.layout.GetFrontPage()

	SetInfoLayout(tui.infoPage.layout, msg, infoType, func() {
		tui.body.layout.SwitchToPage(name)
	})

	tui.messageModalMutex.Unlock()
	tui.body.layout.SwitchToPage("-1")

}

func (tui *tuiConfig) showMessage(msg string) {
	tui.queueUpdateDraw(func() {
		tui.footer.layout.SetText(msg).SetTextColor(tcell.ColorGreen)
	})
	go time.AfterFunc(3*time.Second, tui.resetMessage)
}

func (tui *tuiConfig) showWarning(msg string) {
	tui.showMessageModal(msg, "Warning")
}

func (tui *tuiConfig) showInfo(msg string) {
	tui.showMessageModal(msg, "Info")
}

func (tui *tuiConfig) showError(err error) {
	tui.showMessageModal(err.Error(), "Error")
}
