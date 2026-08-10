// Package settings persists the app's GMLC MLP connection settings under
// the current user's registry hive -- there is no config file; everything
// is entered and edited through the app's own Settings dialog.
package settings

import (
	"golang.org/x/sys/windows/registry"

	"lcsclient/internal/maptile"
)

const keyPath = `Software\VectorCore\LCSClient`

// Settings holds everything needed to talk to a GMLC's MLP (Le) listener
// and to render the result map.
type Settings struct {
	BaseURL        string
	ClientID       string
	Password       string
	TimeoutSeconds int
	// TileURLTemplate is a fmt-style "%d/%d/%d" (zoom/x/y) URL for the map's
	// tile source. Defaults to the public OpenStreetMap tile server; point
	// it at an internal or paid tile server for heavier use.
	TileURLTemplate string
}

// Default returns the settings a freshly installed app starts with.
func Default() Settings {
	return Settings{TimeoutSeconds: 10, TileURLTemplate: maptile.DefaultURLTemplate}
}

// Load reads settings from HKCU\Software\VectorCore\LCSClient, falling
// back to Default for any value that isn't set yet (e.g. first run).
func Load() (Settings, error) {
	s := Default()

	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return s, nil
		}
		return s, err
	}
	defer k.Close()

	if v, _, err := k.GetStringValue("BaseURL"); err == nil {
		s.BaseURL = v
	}
	if v, _, err := k.GetStringValue("ClientID"); err == nil {
		s.ClientID = v
	}
	if v, _, err := k.GetStringValue("Password"); err == nil {
		s.Password = v
	}
	if v, _, err := k.GetIntegerValue("TimeoutSeconds"); err == nil {
		s.TimeoutSeconds = int(v)
	}
	if v, _, err := k.GetStringValue("TileURLTemplate"); err == nil && v != "" {
		s.TileURLTemplate = v
	}
	return s, nil
}

// Save writes s to HKCU\Software\VectorCore\LCSClient, creating the key
// if this is the first time the app has saved settings.
func Save(s Settings) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	if err := k.SetStringValue("BaseURL", s.BaseURL); err != nil {
		return err
	}
	if err := k.SetStringValue("ClientID", s.ClientID); err != nil {
		return err
	}
	if err := k.SetStringValue("Password", s.Password); err != nil {
		return err
	}
	if err := k.SetDWordValue("TimeoutSeconds", uint32(s.TimeoutSeconds)); err != nil {
		return err
	}
	if err := k.SetStringValue("TileURLTemplate", s.TileURLTemplate); err != nil {
		return err
	}
	return nil
}
