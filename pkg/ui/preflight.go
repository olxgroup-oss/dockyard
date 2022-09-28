package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type preflight struct {
	layout             *tview.Flex
	availableIpFrame   *tview.Frame
	availableIpTable   *tview.Table
	resourceQuotaFrame *tview.Frame
	resourceQuotaTable *tview.Table
	publicImageTable   *tview.Table
	publicImageFrame   *tview.Frame
	pdbFrame           *tview.Frame
	pdbTable           *tview.Table
	focused            bool
}

func NewPreflight() *preflight {

	publicImageTable := tview.NewTable()
	resourceQuotaTableView := tview.NewTable()
	availableIpTableView := tview.NewTable()
	pdbTableView := tview.NewTable()
	publicImageTable.SetBorder(true).SetTitle("PublicImages").SetBorder(true)
	availableIpTableView.SetBorder(true).SetTitle("Subnets").SetBorder(true)
	resourceQuotaTableView.SetBorder(true).
		SetTitle("Health Checks").
		SetBorder(true)
	pdbTableView.SetBorder(true).SetTitle("Misconfigured PDB").SetBorder(true)
	availableIpsFrame := tview.NewFrame(availableIpTableView).
		AddText("Available Ips", true, tview.AlignLeft, tcell.ColorYellow)
	resourceQuotaFrame := tview.NewFrame(resourceQuotaTableView).
		AddText("Health Checks", true, tview.AlignLeft, tcell.ColorYellow)
	pdbTableFrame := tview.NewFrame(pdbTableView).
		AddText("PDB - with no disruption allowed", true, tview.AlignLeft, tcell.ColorYellow)
	publicImageFrame := tview.NewFrame(publicImageTable).
		AddText("Public Images", true, tview.AlignLeft, tcell.ColorYellow)

	layout := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(availableIpsFrame, 0, 3, false).
		AddItem(pdbTableFrame, 0, 3, false).
		AddItem(publicImageFrame, 0, 3, false).
		AddItem(resourceQuotaFrame, 0, 2, false)

	return &preflight{
		layout:             layout,
		availableIpFrame:   tview.NewFrame(availableIpTableView),
		availableIpTable:   availableIpTableView,
		resourceQuotaFrame: tview.NewFrame(resourceQuotaTableView),
		resourceQuotaTable: resourceQuotaTableView,
		pdbFrame:           pdbTableFrame,
		pdbTable:           pdbTableView,
		publicImageTable:   publicImageTable,
		publicImageFrame:   publicImageFrame,
		focused:            false,
	}
}

func (tui *tuiConfig) renderPreflightFlex() {

	tui.queueUpdateDraw(func() {

		defer tui.body.layout.SwitchToPage("2")

		// Initialize preflight table headers
		ips := make([][]string, 0)
		ips = append(ips, []string{"Subnet", "Availability Zone", "Ips"})
		rqs := make([][]string, 0)
		rqs = append(rqs, []string{"Type", "Status"})
		pdbs := make([][]string, 0)
		pdbs = append(pdbs, []string{"PDB Name", "Namespace", "ExpectedPods"})
		publicImages := make([][]string, 0)
		publicImages = append(publicImages, []string{"Deployment", "Namespace"})

		healthStatus := make(chan []string)

		// fetch if all worker nodes are healthy
		go func(healthStatus chan []string) {
			healthy, _ := tui.kube.AreNodeHealthy()
			healthStatus <- healthy
		}(healthStatus)

		// fetch if any pods are in pending state
		go func(healthStatus chan []string) {
			pending, _ := tui.kube.ArePendingPods()
			healthStatus <- pending
		}(healthStatus)

		// fetch count of available ips in each subnet
		res, err := tui.awsEksClient.AvailableIp()
		if err != nil {
			tui.showError(err)
		}
		ips = append(ips, res...)

		// Get EC2 spot limits
		res, err = tui.awsEksClient.Ec2Limits()

		if err != nil {
			tui.showError(err)
		}
		rqs = append(rqs, res...)

		// Get misconfigured PDBs
		res, err = tui.kube.GetPDB()

		if err != nil {
			tui.showError(err)
		}
		pdbs = append(pdbs, res...)

		// Fetch all images which are not pulled of private registry
		res, err = tui.kube.ListPublicImages()

		if err != nil {
			tui.showError(err)
		}

		publicImages = append(publicImages, res...)

		rqs = append(rqs, <-healthStatus)
		rqs = append(rqs, <-healthStatus)

		availableIpsTable := tui.preflightFlex.availableIpTable
		availableIpsTable.SetBorder(true)
		availableIpsTable.SetFixed(1, 1)

		resourceQuotaTable := tui.preflightFlex.resourceQuotaTable
		resourceQuotaTable.SetFixed(1, 1)
		resourceQuotaTable.SetBorder(true)

		pdbTable := tui.preflightFlex.pdbTable
		pdbTable.SetFixed(1, 1)
		pdbTable.SetBorder(true)

		publicImageTable := tui.preflightFlex.publicImageTable
		publicImageTable.SetFixed(1, 1)
		publicImageTable.SetBorder(true)

		renderTable(ips, availableIpsTable)
		renderTable(rqs, resourceQuotaTable)
		renderTable(pdbs, pdbTable)
		renderTable(publicImages, publicImageTable)
	})
}

func renderTable(table [][]string, tableTui *tview.Table) {

	for r := 0; r < len(table); r++ {
		for c := 0; c < len(table[r]); c++ {
			color := tcell.ColorWhite
			align := tview.AlignLeft
			if r < 1 {
				color = tcell.ColorYellow
				align = tview.AlignLeft
			}
			tableTui.SetCell(r, c,
				&tview.TableCell{
					Text:          table[r][c],
					Color:         color,
					NotSelectable: true,
					Align:         align,
				},
			)
		}
	}
}
