package portrait

import "image"

// NormalizedPortrait is the final cropped-and-resized portrait, ready for
// export.
type NormalizedPortrait struct {
	Pixels image.Image
	Width  int
	Height int
}

// Diagnostic carries everything a renderer needs to draw the source image
// with both bounding boxes overlaid, at whatever scale it chooses, plus the
// resulting portrait — the shared view model behind both the PDF export and
// the eventual HTML preview (see Docs/SPEC_trombinoscope.md §9.4).
type Diagnostic struct {
	Source   SourceImage
	FaceBox  BoundingBox
	CropBox  BoundingBox
	Portrait NormalizedPortrait
}
