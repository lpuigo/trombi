package imageio

import (
	"fmt"
	"image"

	"golang.org/x/image/draw"

	"trombi/domain/portrait"
)

// Cropper implements service.Cropper using golang.org/x/image/draw
// (CatmullRom resampling), pure Go with no CGo (see
// Docs/SPEC_trombinoscope.md §9.4) — the stdlib image package has no
// resampling of its own.
type Cropper struct{}

// Crop extracts box from src and resizes it to out in a single pass.
func (Cropper) Crop(src portrait.SourceImage, box portrait.BoundingBox, out portrait.Resolution) (portrait.NormalizedPortrait, error) {
	sr := box.Rectangle.Intersect(src.Pixels.Bounds())
	if sr.Empty() {
		return portrait.NormalizedPortrait{}, fmt.Errorf(
			"imageio: crop box %v does not intersect source image bounds %v", box.Rectangle, src.Pixels.Bounds())
	}

	dst := image.NewRGBA(image.Rect(0, 0, out.Width, out.Height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src.Pixels, sr, draw.Over, nil)

	return portrait.NormalizedPortrait{Pixels: dst, Width: out.Width, Height: out.Height}, nil
}
