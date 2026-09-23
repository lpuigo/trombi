// Package service orchestrates the portrait domain: detect, select, frame,
// crop. It contains no business rules of its own (those live in
// domain/portrait) and no image-decoding or PDF-rendering details (those
// live in infra) — it only sequences the calls and assembles the result.
package service

import "trombi/domain/portrait"

// Cropper crops a SourceImage to box and resizes the result to out. Kept as
// an interface owned by this layer (rather than by domain) because, unlike
// the pure geometry in domain, actual pixel resampling is delegated to an
// image-processing library whose internals the domain has no reason to
// know about.
type Cropper interface {
	Crop(src portrait.SourceImage, box portrait.BoundingBox, out portrait.Resolution) (portrait.NormalizedPortrait, error)
}

// ProcessImage runs the full single-image pipeline: detect faces, select the
// primary one, compute and clamp the crop box, then produce the normalized
// portrait and the diagnostic view model used by exporters.
func ProcessImage(detector portrait.FaceDetector, cropper Cropper, spec portrait.FramingSpec, src portrait.SourceImage) portrait.ProcessingResult {
	faces, err := detector.Detect(src)
	if err != nil {
		return portrait.ProcessingResult{Status: portrait.StatusFailure, SourceImage: src, FailureReason: err}
	}

	selected, warnings, err := portrait.SelectPrimaryFace(faces)
	if err != nil {
		return portrait.ProcessingResult{Status: portrait.StatusFailure, SourceImage: src, DetectedFaces: faces, FailureReason: err}
	}

	return frameAndCrop(cropper, spec, src, faces, selected, warnings)
}

// ReframeImage recomputes the crop box and portrait for an already-detected
// face under a new FramingSpec, without rerunning face detection — the
// operation behind a user editing an Item's framing parameters after the
// batch has already been processed once (see service.ReframeItem).
func ReframeImage(cropper Cropper, spec portrait.FramingSpec, src portrait.SourceImage, faces []portrait.DetectedFace, selected portrait.DetectedFace) portrait.ProcessingResult {
	return frameAndCrop(cropper, spec, src, faces, selected, nil)
}

// frameAndCrop is the shared tail of ProcessImage and ReframeImage: derive
// the crop box from the selected face and spec, clamp it to the source
// image, then crop/resize into the final portrait.
func frameAndCrop(cropper Cropper, spec portrait.FramingSpec, src portrait.SourceImage, faces []portrait.DetectedFace, selected portrait.DetectedFace, warnings []portrait.Warning) portrait.ProcessingResult {
	cropBox := portrait.ComputeCropBox(selected.Box, spec)
	cropBox, clampWarnings, err := portrait.FitWithinBounds(cropBox, selected.Box, src.Width, src.Height)
	if err != nil {
		return portrait.ProcessingResult{Status: portrait.StatusFailure, SourceImage: src, DetectedFaces: faces, SelectedFace: selected, FailureReason: err}
	}
	warnings = append(warnings, clampWarnings...)

	normalized, err := cropper.Crop(src, cropBox, spec.Output)
	if err != nil {
		return portrait.ProcessingResult{Status: portrait.StatusFailure, SourceImage: src, DetectedFaces: faces, SelectedFace: selected, FailureReason: err}
	}

	status := portrait.StatusSuccess
	if len(warnings) > 0 {
		status = portrait.StatusWarning
	}

	return portrait.ProcessingResult{
		Status:        status,
		SourceImage:   src,
		DetectedFaces: faces,
		SelectedFace:  selected,
		Diagnostic: portrait.Diagnostic{
			Source:   src,
			FaceBox:  selected.Box,
			CropBox:  cropBox,
			Portrait: normalized,
		},
		Warnings: warnings,
	}
}
