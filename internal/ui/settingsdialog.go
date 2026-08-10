package ui

import (
	"strings"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"lcsclient/internal/maptile"
	"lcsclient/internal/settings"
)

// showSettingsDialog opens a modal dialog for editing GMLC MLP connection
// settings, seeded from current. It returns the edited settings and true
// if the user clicked Save, or the original settings and false if they
// cancelled.
func showSettingsDialog(owner walk.Form, current settings.Settings) (settings.Settings, bool) {
	result := current

	var dlg *walk.Dialog
	var baseURLEdit, clientIDEdit, passwordEdit, tileURLEdit *walk.LineEdit
	var timeoutEdit *walk.NumberEdit
	var savePB, cancelPB *walk.PushButton

	code, err := Dialog{
		AssignTo:      &dlg,
		Title:         "Settings",
		MinSize:       Size{Width: 460, Height: 260},
		Layout:        VBox{},
		DefaultButton: &savePB,
		CancelButton:  &cancelPB,
		Children: []Widget{
			Composite{
				Layout: Grid{Columns: 2},
				Children: []Widget{
					Label{Text: "GMLC MLP URL:"},
					LineEdit{AssignTo: &baseURLEdit, Text: current.BaseURL, CueBanner: "http://gmlc-host:9210"},

					Label{Text: "Client ID:"},
					LineEdit{AssignTo: &clientIDEdit, Text: current.ClientID},

					Label{Text: "Token:"},
					LineEdit{AssignTo: &passwordEdit, Text: current.Password, PasswordMode: true},

					Label{Text: "Request timeout (seconds):"},
					NumberEdit{AssignTo: &timeoutEdit, Value: float64(current.TimeoutSeconds), MinValue: 1, MaxValue: 120, Decimals: 0},

					Label{Text: "Map tile server URL:"},
					LineEdit{AssignTo: &tileURLEdit, Text: current.TileURLTemplate, CueBanner: maptile.DefaultURLTemplate},
				},
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{
						AssignTo: &savePB,
						Text:     "Save",
						OnClicked: func() {
							baseURL := strings.TrimSpace(baseURLEdit.Text())
							if baseURL == "" {
								walk.MsgBox(dlg, "Missing URL", "GMLC MLP URL is required.", walk.MsgBoxIconWarning)
								return
							}
							result.BaseURL = baseURL
							result.ClientID = strings.TrimSpace(clientIDEdit.Text())
							result.Password = passwordEdit.Text()
							result.TimeoutSeconds = int(timeoutEdit.Value())
							tileURL := strings.TrimSpace(tileURLEdit.Text())
							if tileURL == "" {
								tileURL = maptile.DefaultURLTemplate
							}
							result.TileURLTemplate = tileURL
							dlg.Accept()
						},
					},
					PushButton{
						AssignTo:  &cancelPB,
						Text:      "Cancel",
						OnClicked: func() { dlg.Cancel() },
					},
				},
			},
		},
	}.Run(owner)
	if err != nil {
		return current, false
	}
	if code != walk.DlgCmdOK {
		return current, false
	}
	return result, true
}
