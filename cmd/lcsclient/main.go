// Command lcsclient is a native Windows LCS client: it submits 4G LTE
// location requests to a GMLC over OMA MLP v3.5 (Le interface). All
// settings are entered and edited through the app itself and persisted
// to the current user's registry hive -- there is no config file.
package main

import (
	"github.com/lxn/walk"

	"lcsclient/internal/settings"
	"lcsclient/internal/ui"
)

func main() {
	s, err := settings.Load()
	if err != nil {
		walk.MsgBox(nil, "VectorCore LCS Client", "Failed to load settings: "+err.Error(), walk.MsgBoxIconError)
		return
	}

	app := ui.New(s)
	app.Run()
}
