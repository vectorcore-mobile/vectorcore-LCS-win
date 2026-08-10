// Package ui implements the native Windows GUI for the LCS client, built
// on lxn/walk. There is no config file: every setting is entered and
// edited through the app's own Settings dialog and persisted to the
// registry (see internal/settings).
package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"lcsclient/internal/mlp"
	"lcsclient/internal/settings"
	"lcsclient/internal/version"
)

const appTitle = "VectorCore LCS Client"

// App is the running main window plus everything it needs to submit a
// location request over MLP.
type App struct {
	mw       *walk.MainWindow
	settings settings.Settings
	client   *mlp.Client
	mapView  *mapView

	appIcon *walk.Icon

	imsiRB, msisdnRB         *walk.RadioButton
	targetEdit               *walk.LineEdit
	locTypeCB                *walk.ComboBox
	highPriorityCB           *walk.CheckBox
	useHorAccCB              *walk.CheckBox
	horAccEdit               *walk.NumberEdit
	respTimeCB               *walk.ComboBox
	testPB, submitPB, histPB *walk.PushButton

	stateLabel, latLabel, lonLabel, uncLabel, timeLabel *walk.Label

	statusItem *walk.StatusBarItem
}

// New builds an App from persisted settings without showing it yet.
func New(s settings.Settings) *App {
	a := &App{settings: s, mapView: newMapView(s.TileURLTemplate)}
	a.rebuildClient()

	// Best-effort: a missing/undecodable icon shouldn't stop the app
	// from starting, just leave the title bar/taskbar icon blank.
	a.appIcon, _ = loadEmbeddedIcon(markPNGBytes)

	return a
}

func (a *App) rebuildClient() {
	timeout := time.Duration(a.settings.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	a.client = mlp.NewClient(a.settings.BaseURL, a.settings.ClientID, a.settings.Password, timeout)
}

// Run shows the main window and blocks until it's closed, returning the
// process exit code.
func (a *App) Run() int {
	locTypeOptions := []string{"Current position", "Current or last known"}
	respTimeOptions := []string{"Default", "Low delay", "Delay tolerant"}

	mainWin := MainWindow{
		AssignTo: &a.mw,
		Title:    appTitle,
		Icon:     a.appIcon,
		MinSize:  Size{Width: 560, Height: 760},
		Layout:   VBox{},
		MenuItems: []MenuItem{
			Menu{
				Text: "&File",
				Items: []MenuItem{
					Action{Text: "&Settings...", OnTriggered: a.onSettings},
					Separator{},
					Action{Text: "E&xit", OnTriggered: func() { a.mw.Close() }},
				},
			},
			Menu{
				Text: "&Help",
				Items: []MenuItem{
					Action{Text: "&About", OnTriggered: a.onAbout},
				},
			},
		},
		StatusBarItems: []StatusBarItem{
			{AssignTo: &a.statusItem, Text: "GMLC: not checked"},
		},
		Children: []Widget{
			GroupBox{
				Title:  "Location Request",
				Layout: Grid{Columns: 2},
				Children: []Widget{
					Label{Text: "Target:"},
					Composite{
						Layout: HBox{MarginsZero: true},
						Children: []Widget{
							RadioButton{AssignTo: &a.imsiRB, Text: "IMSI", Value: "imsi"},
							RadioButton{AssignTo: &a.msisdnRB, Text: "MSISDN", Value: "msisdn"},
							HSpacer{},
						},
					},

					Label{Text: "Subscriber ID:"},
					Composite{
						Layout: HBox{MarginsZero: true},
						Children: []Widget{
							LineEdit{AssignTo: &a.targetEdit, CueBanner: "e.g. 310150123456789", MaxLength: 25, MinSize: Size{Width: 190}},
							HSpacer{},
						},
					},

					Label{Text: "Location type:"},
					Composite{
						Layout: HBox{MarginsZero: true},
						Children: []Widget{
							ComboBox{AssignTo: &a.locTypeCB, Model: locTypeOptions, CurrentIndex: 0},
							HSpacer{},
						},
					},

					Label{Text: ""},
					CheckBox{AssignTo: &a.highPriorityCB, Text: "High priority"},

					Label{Text: "Horizontal accuracy (m):"},
					Composite{
						Layout: HBox{MarginsZero: true},
						Children: []Widget{
							CheckBox{AssignTo: &a.useHorAccCB},
							NumberEdit{AssignTo: &a.horAccEdit, MinValue: 0, MaxValue: 9999, Decimals: 0, Value: 100},
							HSpacer{},
						},
					},

					Label{Text: "Response time:"},
					Composite{
						Layout: HBox{MarginsZero: true},
						Children: []Widget{
							ComboBox{AssignTo: &a.respTimeCB, Model: respTimeOptions, CurrentIndex: 0},
							HSpacer{},
						},
					},
				},
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{AssignTo: &a.testPB, Text: "Test Connection", OnClicked: a.onTestConnection},
					PushButton{AssignTo: &a.histPB, Text: "View History...", OnClicked: a.onViewHistory},
					PushButton{AssignTo: &a.submitPB, Text: "Request Location", OnClicked: a.onSubmit},
				},
			},
			GroupBox{
				Title:  "Result",
				Layout: Grid{Columns: 2},
				Children: []Widget{
					Label{Text: "State:"},
					Label{AssignTo: &a.stateLabel, Text: "-"},
					Label{Text: "Latitude:"},
					Label{AssignTo: &a.latLabel, Text: "-"},
					Label{Text: "Longitude:"},
					Label{AssignTo: &a.lonLabel, Text: "-"},
					Label{Text: "Uncertainty (m):"},
					Label{AssignTo: &a.uncLabel, Text: "-"},
					Label{Text: "Fix time (UTC):"},
					Label{AssignTo: &a.timeLabel, Text: "-"},
				},
			},
			GroupBox{
				Title:  "UE Location",
				Layout: VBox{MarginsZero: true},
				Children: []Widget{
					a.mapView.widgetDecl(),
				},
			},
		},
	}

	if err := mainWin.Create(); err != nil {
		walk.MsgBox(nil, appTitle, fmt.Sprintf("Failed to start UI: %v", err), walk.MsgBoxIconError)
		return 1
	}
	a.msisdnRB.SetChecked(true)
	a.horAccEdit.SetValue(100)
	a.mapView.attachEvents()
	return a.mw.Run()
}

func (a *App) onSettings() {
	updated, ok := showSettingsDialog(a.mw, a.settings)
	if !ok {
		return
	}
	if err := settings.Save(updated); err != nil {
		walk.MsgBox(a.mw, appTitle, fmt.Sprintf("Failed to save settings: %v", err), walk.MsgBoxIconError)
		return
	}
	a.settings = updated
	a.rebuildClient()
	a.mapView.setTileURLTemplate(updated.TileURLTemplate)
	a.statusItem.SetText("GMLC: not checked")
}

func (a *App) onAbout() {
	walk.MsgBox(a.mw, "About "+appTitle,
		fmt.Sprintf(
			"A native Windows LCS client for 4G LTE location requests, speaking OMA MLP v3.5 to a GMLC's Le interface.\n\nVersion %s",
			version.Version,
		),
		walk.MsgBoxIconInformation)
}

// onViewHistory opens the History dialog for whichever target ID is
// currently entered, querying the GMLC's Historic Location Immediate
// service (hlir/hlia) -- MLP-only, matching the web client's View
// History feature.
func (a *App) onViewHistory() {
	if a.settings.BaseURL == "" {
		walk.MsgBox(a.mw, appTitle, "Set the GMLC MLP URL in Settings first.", walk.MsgBoxIconWarning)
		return
	}
	target, err := a.target()
	if err != nil {
		walk.MsgBox(a.mw, appTitle, err.Error(), walk.MsgBoxIconWarning)
		return
	}
	showHistoryDialog(a.mw, a.client, target, a.mapView)
}

func (a *App) onTestConnection() {
	if a.settings.BaseURL == "" {
		walk.MsgBox(a.mw, appTitle, "Set the GMLC MLP URL in Settings first.", walk.MsgBoxIconWarning)
		return
	}
	a.testPB.SetEnabled(false)
	a.statusItem.SetText("GMLC: checking...")
	client := a.client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := client.Ready(ctx)
		a.mw.Synchronize(func() {
			a.testPB.SetEnabled(true)
			if err != nil {
				a.statusItem.SetText("GMLC: unreachable")
			} else {
				a.statusItem.SetText("GMLC: reachable")
			}
		})
	}()
}

// target reads the currently selected target kind and value from the
// form.
func (a *App) target() (mlp.Target, error) {
	value := strings.TrimSpace(a.targetEdit.Text())
	if value == "" {
		return mlp.Target{}, fmt.Errorf("enter an IMSI or MSISDN")
	}
	if a.msisdnRB.Checked() {
		return mlp.Target{MSISDN: value}, nil
	}
	return mlp.Target{IMSI: value}, nil
}

func (a *App) locationType() mlp.LocationType {
	if a.locTypeCB.CurrentIndex() == 1 {
		return mlp.LocationTypeCurrentOrLastKnown
	}
	return mlp.LocationTypeCurrent
}

func (a *App) qos() *mlp.QoS {
	var q mlp.QoS
	var any bool
	if a.useHorAccCB.Checked() {
		v := a.horAccEdit.Value()
		q.HorizontalAccuracyMeters = &v
		any = true
	}
	switch a.respTimeCB.CurrentIndex() {
	case 1:
		q.ResponseTime = "low_delay"
		any = true
	case 2:
		q.ResponseTime = "delay_tolerant"
		any = true
	}
	if !any {
		return nil
	}
	return &q
}

func (a *App) onSubmit() {
	if a.settings.BaseURL == "" {
		walk.MsgBox(a.mw, appTitle, "Set the GMLC MLP URL in Settings first.", walk.MsgBoxIconWarning)
		return
	}
	target, err := a.target()
	if err != nil {
		walk.MsgBox(a.mw, appTitle, err.Error(), walk.MsgBoxIconWarning)
		return
	}

	locType := a.locationType()
	highPriority := a.highPriorityCB.Checked()
	qos := a.qos()

	a.submitPB.SetEnabled(false)
	a.setResult("requesting...", "", "", "", "")

	client := a.client
	timeout := time.Duration(a.settings.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		status, err := client.Submit(ctx, target, locType, highPriority, qos)
		a.mw.Synchronize(func() {
			a.submitPB.SetEnabled(true)
			if err != nil {
				a.setResult("error", "", "", "", "")
				a.mapView.clear()
				walk.MsgBox(a.mw, appTitle, "Location request failed: "+err.Error(), walk.MsgBoxIconError)
				return
			}
			if status.State == "failed" {
				a.setResult("failed ("+status.FailureCode+")", "", "", "", "")
				a.mapView.clear()
				return
			}
			r := status.Result
			a.setResult(
				status.State,
				strconv.FormatFloat(r.Latitude, 'f', 6, 64),
				strconv.FormatFloat(r.Longitude, 'f', 6, 64),
				strconv.FormatFloat(r.UncertaintyMeters, 'f', 1, 64),
				r.Time.Format("2006-01-02 15:04:05"),
			)
			a.mapView.setResult(r.Latitude, r.Longitude, r.UncertaintyMeters)
		})
	}()
}

func (a *App) setResult(state, lat, lon, unc, t string) {
	a.stateLabel.SetText(orDash(state))
	a.latLabel.SetText(orDash(lat))
	a.lonLabel.SetText(orDash(lon))
	a.uncLabel.SetText(orDash(unc))
	a.timeLabel.SetText(orDash(t))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
