package portrait

import (
	"errors"
	"math"
)

// ErrFaceExceedsSourceBounds is returned when the detected face's own
// bounding box does not fit within the source image, a pathological case
// that should not occur for a real detector output.
var ErrFaceExceedsSourceBounds = errors.New("portrait: detected face bounding box exceeds source image bounds")

// ErrCropExcludesFace is returned when box could only be fit within the
// source image by shrinking it enough that it no longer fully contains
// faceBox — the crop would cut off part of the face. That is treated as a
// failure rather than a silent "clamped" warning, since the resulting
// portrait would defeat the purpose of the tool.
var ErrCropExcludesFace = errors.New("portrait: crop box could not be fit within source image bounds without excluding the detected face")

// FitWithinBounds adjusts box so that it lies entirely within a
// 0,0-(sourceWidth,sourceHeight) frame, per Docs/SPEC_trombinoscope.md §8:
// translate first (size unchanged), then shrink around its own center
// (preserving its aspect ratio) if translation alone cannot make it fit. It
// fails if faceBox itself does not fit, or if the fitted box would exclude
// part of faceBox.
func FitWithinBounds(box, faceBox BoundingBox, sourceWidth, sourceHeight int) (BoundingBox, []Warning, error) {
	if !faceBox.FitsWithin(sourceWidth, sourceHeight) {
		return BoundingBox{}, nil, ErrFaceExceedsSourceBounds
	}
	if box.FitsWithin(sourceWidth, sourceHeight) {
		return box, nil, nil
	}

	fitted := box
	if scale := shrinkFactor(fitted, sourceWidth, sourceHeight); scale < 1 {
		fitted = fitted.ScaledAboutCenter(scale)
	}
	fitted = translateIntoBounds(fitted, sourceWidth, sourceHeight)

	if !fitted.Contains(faceBox) {
		return BoundingBox{}, nil, ErrCropExcludesFace
	}

	return fitted, []Warning{{Kind: WarningCropClamped}}, nil
}

// shrinkFactor returns the largest scale (<= 1) that makes box fit within
// width x height on both axes.
func shrinkFactor(box BoundingBox, width, height int) float64 {
	scale := 1.0
	if w := box.Width(); w > width {
		scale = math.Min(scale, float64(width)/float64(w))
	}
	if h := box.Height(); h > height {
		scale = math.Min(scale, float64(height)/float64(h))
	}
	return scale
}

// translateIntoBounds shifts box back inside 0,0-(width,height) without
// resizing it, assuming it already fits on both axes.
func translateIntoBounds(box BoundingBox, width, height int) BoundingBox {
	dx, dy := 0, 0
	switch {
	case box.Min.X < 0:
		dx = -box.Min.X
	case box.Max.X > width:
		dx = width - box.Max.X
	}
	switch {
	case box.Min.Y < 0:
		dy = -box.Min.Y
	case box.Max.Y > height:
		dy = height - box.Max.Y
	}
	return box.Translated(dx, dy)
}
