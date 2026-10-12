package imageio

import "trombi/domain/portrait"

// Decoder adapts the package-level Decode function to service.ImageDecoder,
// for dependency injection into service.Workspace.
type Decoder struct{}

// Decode implements service.ImageDecoder.
func (Decoder) Decode(data []byte) (portrait.SourceImage, error) {
	return Decode(data)
}
