package portrait

// DetectedFace is one face found by a FaceDetector, expressed in the
// coordinate space of the SourceImage it was found in.
type DetectedFace struct {
	Box BoundingBox
	// Score is the detector's raw confidence value. It is not a normalized
	// probability (Pigo's score is an unbounded classifier sum) — do not
	// compare it against a fixed universal threshold without calibration.
	Score float32
}
