package ui

import (
	"math"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"lcsclient/internal/maptile"
)

const (
	minZoom     = 2
	maxZoom     = 18
	defaultZoom = 2
)

// mapView draws an OpenStreetMap-tiled map natively (no embedded
// browser) showing the last resolved UE position and its uncertainty
// circle, mirroring what the web client's Leaflet map showed.
type mapView struct {
	widget  *walk.CustomWidget
	fetcher *maptile.Fetcher

	centerLat, centerLon float64
	zoom                 int

	hasResult            bool
	resultLat, resultLon float64
	uncertaintyM         float64

	bitmaps map[maptile.Key]*walk.Bitmap

	dragging                   bool
	dragStartX, dragStartY     int
	dragStartLat, dragStartLon float64

	markerBrush, ringBrush *walk.SolidColorBrush
	markerPen              *walk.CosmeticPen
	bgBrush                *walk.SystemColorBrush
	attrFont               *walk.Font
}

func newMapView(tileURLTemplate string) *mapView {
	return &mapView{
		fetcher:   maptile.NewFetcher(tileURLTemplate),
		centerLat: 20, centerLon: 0, zoom: defaultZoom,
		bitmaps: map[maptile.Key]*walk.Bitmap{},
	}
}

// widgetDecl returns the declarative widget to embed in the main
// window's layout.
func (m *mapView) widgetDecl() CustomWidget {
	return CustomWidget{
		AssignTo:            &m.widget,
		Paint:               m.paint,
		PaintMode:           PaintBuffered,
		InvalidatesOnResize: true,
		MinSize:             Size{Width: 200, Height: 260},
		OnMouseDown:         m.onMouseDown,
		OnMouseMove:         m.onMouseMove,
		OnMouseUp:           m.onMouseUp,
	}
}

// attachEvents wires up events that the declarative CustomWidget struct
// has no field for. Call once after the main window is created.
func (m *mapView) attachEvents() {
	m.widget.MouseWheel().Attach(m.onMouseWheel)
}

// setTileURLTemplate points the map at a different tile server (e.g.
// after the user changes it in Settings).
func (m *mapView) setTileURLTemplate(urlTemplate string) {
	m.fetcher.SetURLTemplate(urlTemplate)
	m.bitmaps = map[maptile.Key]*walk.Bitmap{}
	if m.widget != nil {
		m.widget.Invalidate()
	}
}

// setResult centers/zooms the map on a resolved position, fitting its
// uncertainty circle in view -- the same "fitBounds around the point"
// behavior the web client's Leaflet map had.
func (m *mapView) setResult(lat, lon, uncertaintyM float64) {
	m.hasResult = true
	m.resultLat, m.resultLon, m.uncertaintyM = lat, lon, uncertaintyM
	m.centerLat, m.centerLon = lat, lon
	m.zoom = m.fitZoom(lat, uncertaintyM)
	if m.widget != nil {
		m.widget.Invalidate()
	}
}

// clear removes the marker, leaving the map panned/zoomed where it was.
func (m *mapView) clear() {
	m.hasResult = false
	if m.widget != nil {
		m.widget.Invalidate()
	}
}

func (m *mapView) fitZoom(lat, uncertaintyM float64) int {
	radius := uncertaintyM
	if radius <= 0 {
		radius = 300
	}
	radius = math.Max(radius*2.5, 300)

	width, height := 600.0, 380.0
	if m.widget != nil {
		if b := m.widget.ClientBoundsPixels(); b.Width > 0 && b.Height > 0 {
			width, height = float64(b.Width), float64(b.Height)
		}
	}
	minDim := math.Min(width, height)

	for z := maxZoom; z >= minZoom; z-- {
		mpp := maptile.MetersPerPixel(lat, z)
		diameterPx := (radius / mpp) * 2
		if diameterPx <= minDim*0.8 {
			return z
		}
	}
	return minZoom
}

func (m *mapView) ensureResources() error {
	var err error
	if m.markerBrush == nil {
		if m.markerBrush, err = walk.NewSolidColorBrush(walk.RGB(0xf5, 0xa6, 0x23)); err != nil {
			return err
		}
	}
	if m.ringBrush == nil {
		if m.ringBrush, err = walk.NewSolidColorBrush(walk.RGB(0xf5, 0xe6, 0xc3)); err != nil {
			return err
		}
	}
	if m.markerPen == nil {
		if m.markerPen, err = walk.NewCosmeticPen(walk.PenSolid, walk.RGB(0xf5, 0xa6, 0x23)); err != nil {
			return err
		}
	}
	if m.attrFont == nil {
		if m.attrFont, err = walk.NewFont("Segoe UI", 8, 0); err != nil {
			return err
		}
	}
	if m.bgBrush == nil {
		if m.bgBrush, err = walk.NewSystemColorBrush(walk.SysColor3DFace); err != nil {
			return err
		}
	}
	return nil
}

func (m *mapView) bitmap(key maptile.Key) *walk.Bitmap {
	if bmp, ok := m.bitmaps[key]; ok {
		return bmp
	}
	img, ok := m.fetcher.Get(key, func() {
		if m.widget != nil {
			m.widget.Synchronize(func() {
				m.widget.Invalidate()
			})
		}
	})
	if !ok {
		return nil
	}
	bmp, err := walk.NewBitmapFromImage(img)
	if err != nil {
		return nil
	}
	m.bitmaps[key] = bmp
	return bmp
}

func (m *mapView) paint(canvas *walk.Canvas, _ walk.Rectangle) error {
	if err := m.ensureResources(); err != nil {
		return err
	}

	bounds := m.widget.ClientBoundsPixels()
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return nil
	}

	if err := canvas.FillRectanglePixels(m.bgBrush, bounds); err != nil {
		return err
	}

	centerPxX, centerPxY := maptile.LatLonToWorldPixel(m.centerLat, m.centerLon, m.zoom)
	originX := centerPxX - float64(bounds.Width)/2
	originY := centerPxY - float64(bounds.Height)/2

	numTiles := 1 << uint(m.zoom)
	startTX := int(math.Floor(originX / maptile.TileSize))
	endTX := int(math.Floor((originX + float64(bounds.Width)) / maptile.TileSize))
	startTY := int(math.Floor(originY / maptile.TileSize))
	endTY := int(math.Floor((originY + float64(bounds.Height)) / maptile.TileSize))

	for ty := startTY; ty <= endTY; ty++ {
		if ty < 0 || ty >= numTiles {
			continue
		}
		for tx := startTX; tx <= endTX; tx++ {
			wrappedTX := ((tx % numTiles) + numTiles) % numTiles
			key := maptile.Key{Z: m.zoom, X: wrappedTX, Y: ty}
			dst := walk.Point{
				X: int(float64(tx*maptile.TileSize) - originX),
				Y: int(float64(ty*maptile.TileSize) - originY),
			}
			if bmp := m.bitmap(key); bmp != nil {
				canvas.DrawImagePixels(bmp, dst)
			}
		}
	}

	if m.hasResult {
		markerPxX, markerPxY := maptile.LatLonToWorldPixel(m.resultLat, m.resultLon, m.zoom)
		cx := markerPxX - originX
		cy := markerPxY - originY

		if m.uncertaintyM > 0 {
			mpp := maptile.MetersPerPixel(m.resultLat, m.zoom)
			radiusPx := m.uncertaintyM / mpp
			ringBounds := walk.Rectangle{
				X: int(cx - radiusPx), Y: int(cy - radiusPx),
				Width: int(radiusPx * 2), Height: int(radiusPx * 2),
			}
			canvas.FillEllipsePixels(m.ringBrush, ringBounds)
			canvas.DrawEllipsePixels(m.markerPen, ringBounds)
		}

		const dotRadius = 6
		dotBounds := walk.Rectangle{
			X: int(cx - dotRadius), Y: int(cy - dotRadius),
			Width: dotRadius * 2, Height: dotRadius * 2,
		}
		canvas.FillEllipsePixels(m.markerBrush, dotBounds)
	}

	attribution := "© OpenStreetMap contributors"
	attrBounds := walk.Rectangle{X: 4, Y: bounds.Height - 18, Width: bounds.Width - 8, Height: 16}
	canvas.DrawTextPixels(attribution, m.attrFont, walk.RGB(0x33, 0x33, 0x33), attrBounds, walk.TextRight|walk.TextBottom|walk.TextSingleLine)

	return nil
}

func (m *mapView) onMouseDown(x, y int, button walk.MouseButton) {
	if button&walk.LeftButton == 0 {
		return
	}
	m.dragging = true
	m.dragStartX, m.dragStartY = x, y
	m.dragStartLat, m.dragStartLon = m.centerLat, m.centerLon
}

func (m *mapView) onMouseMove(x, y int, button walk.MouseButton) {
	if !m.dragging {
		return
	}
	if button&walk.LeftButton == 0 {
		m.dragging = false
		return
	}
	startPxX, startPxY := maptile.LatLonToWorldPixel(m.dragStartLat, m.dragStartLon, m.zoom)
	newPxX := startPxX - float64(x-m.dragStartX)
	newPxY := startPxY - float64(y-m.dragStartY)
	m.centerLat, m.centerLon = maptile.WorldPixelToLatLon(newPxX, newPxY, m.zoom)
	if m.widget != nil {
		m.widget.Invalidate()
	}
}

func (m *mapView) onMouseUp(x, y int, button walk.MouseButton) {
	m.dragging = false
}

func (m *mapView) onMouseWheel(x, y int, button walk.MouseButton) {
	delta := walk.MouseWheelEventDelta(button)
	switch {
	case delta > 0 && m.zoom < maxZoom:
		m.zoom++
	case delta < 0 && m.zoom > minZoom:
		m.zoom--
	default:
		return
	}
	if m.widget != nil {
		m.widget.Invalidate()
	}
}
