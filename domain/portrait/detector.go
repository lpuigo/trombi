package portrait

// FaceDetector locates faces in a SourceImage. It is the one interface owned
// by the domain layer: Docs/SPEC_trombinoscope.md §7/§12 explicitly flag the
// detection library (Pigo) as a choice that may need revisiting after
// real-world testing, which is what earns it the abstraction.
//
// Implementations live in infra (e.g. a Pigo-backed detector) and must
// always report each DetectedFace.Box in the SourceImage's own
// full-resolution coordinate space, even if they downscale internally for
// performance.
type FaceDetector interface {
	Detect(SourceImage) ([]DetectedFace, error)
}
