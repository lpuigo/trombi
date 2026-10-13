package portrait

import (
	"errors"
	"math"
)

var (
	// ErrNegativeMargin is returned for a negative top or bottom margin.
	ErrNegativeMargin = errors.New("portrait: framing margins cannot be negative")
	// ErrInvalidAspectRatio is returned for a ratio with a non-positive term.
	ErrInvalidAspectRatio = errors.New("portrait: aspect ratio terms must be strictly positive")
	// ErrInvalidResolution is returned for a non-positive output size.
	ErrInvalidResolution = errors.New("portrait: output resolution must be strictly positive")
)

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
// The face's vertical position in the frame is not a separate setting here
// — it is a consequence of these two margins, and would otherwise
// over-determine the crop rectangle's height (see Docs/SPEC_trombinoscope.md
// §9.2 for the full rationale). HorizontalOffset, by contrast, is an
// independent degree of freedom: it only moves the crop box sideways and
// never changes its width or height, so it cannot over-determine anything
// — see §9.2 (revised 2026-10-13) for why it was added as a real setting
// rather than staying a derived, display-only value.
type FramingSpec struct {
	TopMargin    float64 // fraction of face height added above the face
	BottomMargin float64 // fraction of face height added below the face
	// HorizontalOffset shifts the crop box sideways from the face's
	// horizontal center, as a fraction of face width: positive moves it
	// right, negative left. 0 keeps the box centered on the face (the
	// previous, implicit behavior).
	HorizontalOffset float64
	Ratio            AspectRatio
	Output           Resolution
}

// DefaultFramingSpec returns the indicative defaults from
// Docs/SPEC_trombinoscope.md §9.2.
func DefaultFramingSpec() FramingSpec {
	return FramingSpec{
		TopMargin:        0.30,
		BottomMargin:     0.47,
		HorizontalOffset: 0,
		Ratio:            AspectRatio{Width: 3, Height: 4},
		Output:           Resolution{Width: 600, Height: 800},
	}
}

// Validate checks the spec's invariants; every FramingSpec coming from user
// input (CLI flags, web form) must pass it before reaching ComputeCropBox.
func (s FramingSpec) Validate() error {
	switch {
	case s.TopMargin < 0 || s.BottomMargin < 0:
		return ErrNegativeMargin
	case s.Ratio.Width <= 0 || s.Ratio.Height <= 0:
		return ErrInvalidAspectRatio
	case s.Output.Width <= 0 || s.Output.Height <= 0:
		return ErrInvalidResolution
	default:
		return nil
	}
}

// WithFraming returns a copy of s with its margins and horizontal offset
// replaced. These three are the only framing parameters a single portrait
// may override: ratio and resolution stay batch-wide so every cell of the
// trombinoscope keeps the same shape (Docs/SPEC_trombinoscope.md §10.2).
func (s FramingSpec) WithFraming(top, bottom, horizontalOffset float64) FramingSpec {
	s.TopMargin = top
	s.BottomMargin = bottom
	s.HorizontalOffset = horizontalOffset
	return s
}

// HorizontalMargin returns the baseline margin on each side of the face a
// centered crop box would have (i.e. with HorizontalOffset at 0), as a
// fraction of the face width — a derived, display-only value
// (Docs/SPEC_trombinoscope.md §9.2). A non-zero HorizontalOffset makes the
// actual left/right margins asymmetric; this value stays their average. It
// assumes a square face box, which is what the Pigo detector always returns
// (§7).
func (s FramingSpec) HorizontalMargin() float64 {
	cropWidth := (1 + s.TopMargin + s.BottomMargin) * s.Ratio.Value()
	return (cropWidth - 1) / 2
}

// FaceVerticalPosition returns where the face's center sits in the crop,
// as a fraction of the crop height from its top — a derived, display-only
// value (Docs/SPEC_trombinoscope.md §9.2).
func (s FramingSpec) FaceVerticalPosition() float64 {
	return (s.TopMargin + 0.5) / (1 + s.TopMargin + s.BottomMargin)
}

// ComputeCropBox derives the crop box for a detected face box: the face's
// own height plus the margins determines the crop height, and the crop
// width is derived from the output ratio rather than from an independent
// horizontal margin. The box is centered on the face horizontally, then
// shifted sideways by HorizontalOffset.
func ComputeCropBox(face BoundingBox, spec FramingSpec) BoundingBox {
	faceHeight := float64(face.Height())
	cropHeight := faceHeight * (1 + spec.TopMargin + spec.BottomMargin)
	cropWidth := cropHeight * spec.Ratio.Value()

	top := float64(face.Min.Y) - spec.TopMargin*faceHeight
	left := face.CenterX() + spec.HorizontalOffset*float64(face.Width()) - cropWidth/2

	return NewBoundingBox(
		int(math.Round(left)), int(math.Round(top)),
		int(math.Round(left+cropWidth)), int(math.Round(top+cropHeight)),
	)
}
