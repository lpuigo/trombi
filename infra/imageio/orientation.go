package imageio

import "image"

// applyOrientation returns src re-oriented according to an EXIF Orientation
// tag value (1-8), so that later processing (face detection, cropping)
// always sees an upright image. Values outside 1-8, or 1 itself, are a
// no-op.
func applyOrientation(src image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return src
	}

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()

	swapped := o >= 5
	dstW, dstH := w, h
	if swapped {
		dstW, dstH = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))

	for y := range h {
		for x := range w {
			var dx, dy int
			switch o {
			case 2: // mirror horizontal
				dx, dy = w-1-x, y
			case 3: // rotate 180
				dx, dy = w-1-x, h-1-y
			case 4: // mirror vertical
				dx, dy = x, h-1-y
			case 5: // mirror horizontal + rotate 270 CW
				dx, dy = y, x
			case 6: // rotate 90 CW
				dx, dy = h-1-y, x
			case 7: // mirror horizontal + rotate 90 CW
				dx, dy = h-1-y, w-1-x
			case 8: // rotate 270 CW (90 CCW)
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
