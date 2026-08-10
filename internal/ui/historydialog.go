package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"lcsclient/internal/mlp"
)

// showHistoryDialog queries the GMLC's Historic Location Immediate
// service (hlir/hlia) for target's recorded fixes in a chosen window and
// lists them -- the MLP-only "View History" feature the web client has
// (MLP's Standard Location Immediate service has no history of its own;
// this is a separate query). Selecting a fix and clicking "Show on Map"
// (or double-clicking it) plots that point on mv, the same map the main
// window's location results show up on.
func showHistoryDialog(owner walk.Form, client *mlp.Client, target mlp.Target, mv *mapView) {
	targetLabel := target.IMSI
	if targetLabel == "" {
		targetLabel = target.MSISDN
	}

	var dlg *walk.Dialog
	var fromEdit, toEdit *walk.DateEdit
	var queryPB, showOnMapPB, closePB *walk.PushButton
	var resultsLB *walk.ListBox
	var statusLbl *walk.Label
	var currentPoints []mlp.HistoryPoint

	showSelected := func() {
		idx := resultsLB.CurrentIndex()
		if idx < 0 || idx >= len(currentPoints) {
			walk.MsgBox(dlg, "Show on Map", "Select a fix from the list first.", walk.MsgBoxIconWarning)
			return
		}
		p := currentPoints[idx]
		mv.setResult(p.Latitude, p.Longitude, p.UncertaintyMeters)
	}

	runQuery := func() {
		start := fromEdit.Date()
		stop := toEdit.Date() // zero value (none checked) means "up to now"

		queryPB.SetEnabled(false)
		statusLbl.SetText("Querying...")
		currentPoints = nil
		resultsLB.SetModel([]string{})

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			points, err := client.History(ctx, target, start, stop)
			dlg.Synchronize(func() {
				queryPB.SetEnabled(true)
				if err != nil {
					statusLbl.SetText("Error: " + err.Error())
					return
				}
				if len(points) == 0 {
					statusLbl.SetText("No recorded fixes in this window.")
					return
				}
				currentPoints = points
				lines := make([]string, len(points))
				for i, p := range points {
					lines[i] = fmt.Sprintf("%s   lat %.6f   lon %.6f   ±%.0fm",
						p.RecordedAt.Format("2006-01-02 15:04:05"), p.Latitude, p.Longitude, p.UncertaintyMeters)
				}
				resultsLB.SetModel(lines)
				statusLbl.SetText(fmt.Sprintf("%d fix(es) found. Select one and click \"Show on Map\".", len(points)))
			})
		}()
	}

	Dialog{
		AssignTo:      &dlg,
		Title:         "Location History - " + targetLabel,
		MinSize:       Size{Width: 480, Height: 380},
		Layout:        VBox{},
		DefaultButton: &queryPB,
		CancelButton:  &closePB,
		Children: []Widget{
			Composite{
				Layout: Grid{Columns: 2},
				Children: []Widget{
					Label{Text: "From:"},
					DateEdit{AssignTo: &fromEdit, Date: time.Now().AddDate(0, 0, -7)},

					Label{Text: "To:"},
					DateEdit{AssignTo: &toEdit, Optional: true},
				},
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{AssignTo: &queryPB, Text: "Query", OnClicked: runQuery},
				},
			},
			Label{AssignTo: &statusLbl, Text: "Pick a window and click Query."},
			ListBox{AssignTo: &resultsLB, Model: []string{}, OnItemActivated: showSelected},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{AssignTo: &showOnMapPB, Text: "Show on Map", OnClicked: showSelected},
					HSpacer{},
					PushButton{AssignTo: &closePB, Text: "Close", OnClicked: func() { dlg.Cancel() }},
				},
			},
		},
	}.Run(owner)
}
