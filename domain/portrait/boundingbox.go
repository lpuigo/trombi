package portrait

import (
	"image"
	"math"
)

// BoundingBox is a rectangle expressed in the pixel coordinate space of a
// SourceImage at its full resolution. It is the one geometry type used both
// for a detected face's native box and for the derived crop box — there is
// deliberately no separate "crop box" type.
type BoundingBox struct {
	image.Rectangle
}

// NewBoundingBox builds a BoundingBox from its corner coordinates.
func NewBoundingBox(x0, y0, x1, y1 int) BoundingBox {
	return BoundingBox{image.Rect(x0, y0, x1, y1)}
}

func (b BoundingBox) Width() int  { return b.Dx() }
func (b BoundingBox) Height() int { return b.Dy() }

func (b BoundingBox) Area() int { return b.Width() * b.Height() }

func (b BoundingBox) CenterX() float64 { return float64(b.Min.X+b.Max.X) / 2 }
func (b BoundingBox) CenterY() float64 { return float64(b.Min.Y+b.Max.Y) / 2 }

// Translated returns b shifted by (dx, dy); its size is unchanged.
func (b BoundingBox) Translated(dx, dy int) BoundingBox {
	return BoundingBox{b.Rectangle.Add(image.Pt(dx, dy))}
}

// ScaledAboutCenter returns b resized by factor around its own center. Width
// is rounded first; height is then derived from it using b's own (pre-scale)
// aspect ratio, rather than rounding both dimensions independently — that
// would let two separate roundings drift the result a pixel or two away
// from b's ratio, which the exported PDF's overlay would then visibly show.
func (b BoundingBox) ScaledAboutCenter(factor float64) BoundingBox {
	cx, cy := b.CenterX(), b.CenterY()
	ratio := float64(b.Width()) / float64(b.Height())

	w := math.Round(float64(b.Width()) * factor)
	h := math.Round(w / ratio)

	minX := math.Round(cx - w/2)
	minY := math.Round(cy - h/2)
	return NewBoundingBox(int(minX), int(minY), int(minX+w), int(minY+h))
}

// FitsWithin reports whether b lies entirely within a 0,0-(width,height) frame.
func (b BoundingBox) FitsWithin(width, height int) bool {
	return b.Min.X >= 0 && b.Min.Y >= 0 && b.Max.X <= width && b.Max.Y <= height
}

// Contains reports whether other lies entirely within b.
func (b BoundingBox) Contains(other BoundingBox) bool {
	return other.Rectangle.In(b.Rectangle)
}
