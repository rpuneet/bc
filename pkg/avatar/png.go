package avatar

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// PNG renders the agent's character to a size×size PNG. It rasterizes the exact
// same SVG that SVG() returns — one generator, so the PNG and SVG are the same
// picture — using a pure-Go rasterizer (no cgo, no external tools). Slack needs
// a raster icon_url, so this is what gets published for the public avatar.
//
// Deterministic: same (name, size) → identical bytes.
func PNG(name string, size int) ([]byte, error) {
	if size <= 0 {
		size = 256
	}
	svg := SVG(name, size)

	icon, err := oksvg.ReadIconStream(strings.NewReader(svg), oksvg.WarnErrorMode)
	if err != nil {
		return nil, fmt.Errorf("avatar: parse svg: %w", err)
	}
	// The SVG declares a 0..64 viewBox with width/height=size; drive the target
	// bounds off size so the character fills the raster.
	icon.SetTarget(0, 0, float64(size), float64(size))

	img := image.NewRGBA(image.Rect(0, 0, size, size))
	scanner := rasterx.NewScannerGV(size, size, img, img.Bounds())
	raster := rasterx.NewDasher(size, size, scanner)
	icon.Draw(raster, 1.0)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("avatar: encode png: %w", err)
	}
	return buf.Bytes(), nil
}
