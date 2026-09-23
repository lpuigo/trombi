package service

import "trombi/domain/portrait"

// ImageLoader decodes a source image from disk. Kept as an interface owned
// by this layer (mirroring Cropper) so the batch pipeline depends on an
// abstraction rather than importing infra/imageio directly.
type ImageLoader interface {
	Load(path string) (portrait.SourceImage, error)
}

// ProcessBatch runs ProcessItem over every Item in b and returns a new
// Batch with updated Results; b itself is not modified.
func ProcessBatch(detector portrait.FaceDetector, cropper Cropper, loader ImageLoader, b portrait.Batch) portrait.Batch {
	processed := make([]portrait.Item, len(b.Items))
	for i, item := range b.Items {
		processed[i] = ProcessItem(detector, cropper, loader, item)
	}
	return portrait.Batch{DefaultSpec: b.DefaultSpec, Items: processed}
}

// ProcessItem loads item's source image and runs the full pipeline under
// item.Spec. A load failure is isolated into the Item's Result exactly like
// a detection or framing failure — it never stops the rest of the batch
// (Docs/SPEC_trombinoscope.md §8/§11).
func ProcessItem(detector portrait.FaceDetector, cropper Cropper, loader ImageLoader, item portrait.Item) portrait.Item {
	src, err := loader.Load(item.SourcePath)
	if err != nil {
		item.Result = portrait.ProcessingResult{Status: portrait.StatusFailure, FailureReason: err}
		return item
	}
	item.Result = ProcessImage(detector, cropper, item.Spec, src)
	return item
}

// ReframeItem recomputes item's crop box and portrait under a new
// FramingSpec, reusing the faces already found by a previous ProcessItem
// call instead of rerunning detection — the operation behind a user editing
// one Item's framing parameters in the UI (Docs/SPEC_trombinoscope.md §9.3).
// It fails with portrait.ErrNoFaceDetected if item has no prior detection to
// reframe.
func ReframeItem(cropper Cropper, spec portrait.FramingSpec, item portrait.Item) (portrait.Item, error) {
	if len(item.Result.DetectedFaces) == 0 {
		return item, portrait.ErrNoFaceDetected
	}
	item.Spec = spec
	item.Result = ReframeImage(cropper, spec, item.Result.SourceImage, item.Result.DetectedFaces, item.Result.SelectedFace)
	return item, nil
}
