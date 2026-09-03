package portrait

import "math"

// AspectRatio is a width:height output ratio.
type AspectRatio struct {
	Width  float64
	Height float64
}

// Value returns the ratio as a single width/height factor.
func (r AspectRatio) Value() float64 { return r.Width / r.Height }

// Resolution is the pixel size of the exported portrait.
type Resolution struct {
	Width  int
	Height int
}

// FramingSpec configures how a crop box is derived from a detected face.
//
// TopMargin and BottomMargin are the canonical parameters: they fix the crop
// box's height relative to the face. Ratio then fixes the crop box's width.
// Horizontal margin and the face's vertical position in the frame are not
// separate settings here — both are consequences of these two margins plus
// Ratio, and would otherwise over-determine the crop rectangle (see
// Docs/SPEC_trombinoscope.md §9.2 for the full rationale).
type FramingSpec struct {
	TopMargin    float64 // fraction of face height added above the face
	BottomMargin float64 // fraction of face height added below the face
	Ratio        AspectRatio
	Output       Resolution
}

// DefaultFramingSpec returns the indicative defaults from
// Docs/SPEC_trombinoscope.md §9.2.
func DefaultFramingSpec() FramingSpec {
	return FramingSpec{
		TopMargin:    0.35,
		BottomMargin: 0.55,
		Ratio:        AspectRatio{Width: 3, Height: 4},
		Output:       Resolution{Width: 600, Height: 800},
	}
}

// ComputeCropBox derives the crop box for a detected face box: the face's
// own height plus the margins determines the crop height, and the crop
// width is derived from the output ratio rather than from an independent
// horizontal margin.
func ComputeCropBox(face BoundingBox, spec FramingSpec) BoundingBox {
	faceHeight := float64(face.Height())
	cropHeight := faceHeight * (1 + spec.TopMargin + spec.BottomMargin)
	cropWidth := cropHeight * spec.Ratio.Value()

	top := float64(face.Min.Y) - spec.TopMargin*faceHeight
	left := face.CenterX() - cropWidth/2

	return NewBoundingBox(
		int(math.Round(left)), int(math.Round(top)),
		int(math.Round(left+cropWidth)), int(math.Round(top+cropHeight)),
	)
}
