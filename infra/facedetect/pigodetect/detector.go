// Package pigodetect implements portrait.FaceDetector on top of Pigo, a
// pure-Go face detector (no CGo, no external cascade file to install — see
// Docs/SPEC_trombinoscope.md §7).
package pigodetect

import (
	_ "embed"
	"fmt"

	pigo "github.com/esimov/pigo/core"

	"trombi/domain/portrait"
)

//go:embed facefinder
var cascadeFile []byte

// Detector is a portrait.FaceDetector backed by Pigo. Tuning values mirror
// Pigo's own reference CLI defaults (cmd/pigo).
type Detector struct {
	classifier   *pigo.Pigo
	minSize      int
	maxSize      int
	shiftFactor  float64
	scaleFactor  float64
	iouThreshold float64
}

// NewDetector unpacks the embedded cascade once and returns a ready-to-use Detector.
func NewDetector() (*Detector, error) {
	classifier, err := pigo.NewPigo().Unpack(cascadeFile)
	if err != nil {
		return nil, fmt.Errorf("pigodetect: unpacking embedded cascade: %w", err)
	}
	return &Detector{
		classifier:   classifier,
		minSize:      20,
		maxSize:      1000,
		shiftFactor:  0.15,
		scaleFactor:  1.15,
		iouThreshold: 0.15,
	}, nil
}

// Detect implements portrait.FaceDetector. Overlapping detections of the
// same real face are merged (ClusterDetections) before being reported, so
// that domain-level ambiguity handling only ever sees distinct faces.
func (d *Detector) Detect(src portrait.SourceImage) ([]portrait.DetectedFace, error) {
	pixels := pigo.RgbToGrayscale(src.Pixels)
	cParams := pigo.CascadeParams{
		MinSize:     d.minSize,
		MaxSize:     d.maxSize,
		ShiftFactor: d.shiftFactor,
		ScaleFactor: d.scaleFactor,
		ImageParams: pigo.ImageParams{
			Pixels: pixels,
			Rows:   src.Height,
			Cols:   src.Width,
			Dim:    src.Width,
		},
	}

	dets := d.classifier.RunCascade(cParams, 0.0)
	dets = d.classifier.ClusterDetections(dets, d.iouThreshold)

	faces := make([]portrait.DetectedFace, 0, len(dets))
	for _, det := range dets {
		half := det.Scale / 2
		box := portrait.NewBoundingBox(det.Col-half, det.Row-half, det.Col+half, det.Row+half)
		faces = append(faces, portrait.DetectedFace{Box: box, Score: det.Q})
	}
	return faces, nil
}
