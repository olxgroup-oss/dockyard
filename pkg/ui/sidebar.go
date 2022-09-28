package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type sidebar struct {
	layout  *sidebarList
	focused bool
}

type sidebarList struct {
	asgRolloutTree *tview.TreeView
}

func NewSidebar() *sidebar {
	return &sidebar{
		layout:  NewSidebarList(),
		focused: false,
	}
}

// Initialize tree node for the sidebar
func NewSidebarList() *sidebarList {
	workerUpgrade := tview.NewTreeView()
	workerUpgrade.SetBorder(true)
	root := tview.NewTreeNode("Worker Node upgrade").SetColor(tcell.ColorYellow)
	workerUpgrade.SetRoot(root).SetCurrentNode(root)
	options := []string{"ASG Rollouts", "Preflight checks"}

	for _, option := range options {
		root.AddChild(
			tview.NewTreeNode(option).SetSelectable(true).SetReference(option),
		)
	}
	return &sidebarList{asgRolloutTree: workerUpgrade}
}

func (list *sidebarList) DisableSelection() {
	rootNode := list.asgRolloutTree.GetRoot()
	for _, child := range rootNode.GetChildren() {
		child.SetSelectable(false)
	}
}
func (list *sidebarList) EnableSelection() {
	rootNode := list.asgRolloutTree.GetRoot()
	for _, child := range rootNode.GetChildren() {
		child.SetSelectable(true)
	}
}
