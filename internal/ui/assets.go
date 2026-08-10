package ui

import (
	"bytes"
	_ "embed"
	"image/png"

	"github.com/lxn/walk"
)

//go:embed assets/mark.png
var markPNGBytes []byte

func loadEmbeddedIcon(data []byte) (*walk.Icon, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return walk.NewIconFromImage(img)
}
