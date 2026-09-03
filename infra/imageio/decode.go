// Package imageio handles all image file I/O: decoding source images
// (with EXIF orientation correction) and cropping/resizing them into a
// portrait.NormalizedPortrait.
package imageio

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"trombi/domain/portrait"
)

// Load decodes an image file from disk into a portrait.SourceImage. For a
// JPEG file, it also reads and applies the EXIF Orientation tag if present
// (see Docs/SPEC_trombinoscope.md §7/§8): the stdlib image/jpeg decoder
// ignores it, which would otherwise make a sideways phone photo fail face
// detection with no visible cause.
func Load(path string) (portrait.SourceImage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return portrait.SourceImage{}, fmt.Errorf("imageio: reading %s: %w", path, err)
	}

	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return portrait.SourceImage{}, fmt.Errorf("imageio: decoding %s: %w", path, err)
	}

	if format == "jpeg" {
		if o, ok := readJPEGOrientation(data); ok {
			img = applyOrientation(img, o)
		}
	}

	return portrait.NewSourceImage(img), nil
}
