package imageio

import "trombi/domain/portrait"

// Loader adapts the package-level Load function to service.ImageLoader, for
// dependency injection into the batch pipeline.
type Loader struct{}

// Load implements service.ImageLoader.
func (Loader) Load(path string) (portrait.SourceImage, error) {
	return Load(path)
}
