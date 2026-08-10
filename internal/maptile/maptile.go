// Package maptile fetches and caches OpenStreetMap-compatible raster
// tiles (Web Mercator, 256px, the standard "slippy map" scheme) so the
// UI can draw a real map natively, without an embedded browser.
package maptile

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"net/http"
	"sync"
	"time"
)

// TileSize is the pixel width/height of one tile at any zoom level.
const TileSize = 256

// DefaultURLTemplate is the standard OpenStreetMap tile server. Its usage
// policy (https://operations.osmfoundation.org/policies/tiles/) expects a
// identifying User-Agent and light, interactive traffic -- fine for this
// app's occasional single-point lookups. Point Settings at an internal or
// paid tile server instead for heavier use.
const DefaultURLTemplate = "https://tile.openstreetmap.org/%d/%d/%d.png"

// LatLonToWorldPixel converts a WGS84 coordinate to a pixel offset in the
// whole rendered world map at the given zoom level (Web Mercator).
func LatLonToWorldPixel(lat, lon float64, zoom int) (x, y float64) {
	n := math.Exp2(float64(zoom)) * TileSize
	x = (lon + 180.0) / 360.0 * n

	latRad := lat * math.Pi / 180.0
	y = (1.0 - math.Log(math.Tan(latRad)+1.0/math.Cos(latRad))/math.Pi) / 2.0 * n
	return x, y
}

// WorldPixelToLatLon is the inverse of LatLonToWorldPixel.
func WorldPixelToLatLon(x, y float64, zoom int) (lat, lon float64) {
	n := math.Exp2(float64(zoom)) * TileSize
	lon = x/n*360.0 - 180.0
	latRad := math.Atan(math.Sinh(math.Pi * (1 - 2*y/n)))
	lat = latRad * 180.0 / math.Pi
	return lat, lon
}

// MetersPerPixel approximates ground resolution at lat/zoom, used to size
// the uncertainty circle and to pick a zoom level that fits it.
func MetersPerPixel(lat float64, zoom int) float64 {
	return 156543.03392 * math.Cos(lat*math.Pi/180.0) / math.Exp2(float64(zoom))
}

// Key identifies one tile.
type Key struct {
	Z, X, Y int
}

// Fetcher downloads and caches tiles. It never evicts -- fine for a tool
// used for occasional single-point lookups, not for open-ended map
// browsing.
type Fetcher struct {
	mu          sync.Mutex
	urlTemplate string
	client      *http.Client
	cache       map[Key]image.Image
	pending     map[Key]bool
}

// NewFetcher returns a Fetcher pulling tiles from urlTemplate (a
// fmt-style "%d/%d/%d" for zoom/x/y), or DefaultURLTemplate if empty.
func NewFetcher(urlTemplate string) *Fetcher {
	if urlTemplate == "" {
		urlTemplate = DefaultURLTemplate
	}
	return &Fetcher{
		urlTemplate: urlTemplate,
		client:      &http.Client{Timeout: 15 * time.Second},
		cache:       map[Key]image.Image{},
		pending:     map[Key]bool{},
	}
}

// SetURLTemplate switches tile servers, dropping anything already
// cached from the old one.
func (f *Fetcher) SetURLTemplate(urlTemplate string) {
	if urlTemplate == "" {
		urlTemplate = DefaultURLTemplate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if urlTemplate == f.urlTemplate {
		return
	}
	f.urlTemplate = urlTemplate
	f.cache = map[Key]image.Image{}
}

// Get returns a cached tile if present. Otherwise it kicks off an async
// fetch (unless one for key is already in flight) and calls onLoaded
// once the tile is cached, so the caller can trigger a repaint.
func (f *Fetcher) Get(key Key, onLoaded func()) (image.Image, bool) {
	f.mu.Lock()
	if img, ok := f.cache[key]; ok {
		f.mu.Unlock()
		return img, true
	}
	if f.pending[key] {
		f.mu.Unlock()
		return nil, false
	}
	f.pending[key] = true
	urlTemplate := f.urlTemplate
	client := f.client
	f.mu.Unlock()

	go func() {
		img, err := fetchTile(client, urlTemplate, key)

		f.mu.Lock()
		delete(f.pending, key)
		if err == nil {
			f.cache[key] = img
		}
		f.mu.Unlock()

		if err == nil && onLoaded != nil {
			onLoaded()
		}
	}()
	return nil, false
}

func fetchTile(client *http.Client, urlTemplate string, key Key) (image.Image, error) {
	url := fmt.Sprintf(urlTemplate, key.Z, key.X, key.Y)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "VectorCore-LCS-Client/0.1 (native Windows MLP location client)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tile fetch %s: %s", url, resp.Status)
	}
	return png.Decode(resp.Body)
}
