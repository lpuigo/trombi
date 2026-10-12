package imageio

import (
	"fmt"
	"image"
	"image/jpeg"
	"io"

	"golang.org/x/image/draw"
)

// JPEGEncoder implements service.ImageEncoder: it writes images as JPEG,
// optionally downscaled for on-screen display.
type JPEGEncoder struct {
	Quality int
}

// Encode writes img to w as a JPEG. If maxDim > 0 and img is larger than
// maxDim on either side, it is first downscaled to fit, preserving its
// aspect ratio. Downscaling uses ApproxBiLinear rather than CatmullRom: the
// result is a screen preview, where speed matters more than fidelity.
func (e JPEGEncoder) Encode(w io.Writer, img image.Image, maxDim int) error {
	if maxDim > 0 {
		img = fitWithin(img, maxDim)
	}
	if err := jpeg.Encode(w, img, &jpeg.Options{Quality: e.Quality}); err != nil {
		return fmt.Errorf("imageio: encoding JPEG: %w", err)
	}
	return nil
}

func fitWithin(img image.Image, maxDim int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxDim && h <= maxDim {
		return img
	}
	if w >= h {
		w, h = maxDim, h*maxDim/w
	} else {
		w, h = w*maxDim/h, maxDim
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}
