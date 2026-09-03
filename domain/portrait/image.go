package portrait

import "image"

// SourceImage is a decoded input image together with its pixel dimensions,
// expressed in its own full-resolution coordinate space.
type SourceImage struct {
	Pixels image.Image
	Width  int
	Height int
}

// NewSourceImage wraps an already-decoded image, deriving its dimensions
// from its bounds.
func NewSourceImage(pixels image.Image) SourceImage {
	b := pixels.Bounds()
	return SourceImage{Pixels: pixels, Width: b.Dx(), Height: b.Dy()}
}
