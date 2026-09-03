package portrait

// WarningKind identifies why processing needed an automatic decision that a
// human might want to double check. Presentation (CLI, GUI...) is
// responsible for turning a Warning into user-facing text; the domain layer
// only records what happened.
type WarningKind int

const (
	// WarningMultipleFacesDetected: more than one face survived clustering;
	// the largest was selected automatically (see SelectPrimaryFace).
	WarningMultipleFacesDetected WarningKind = iota
	// WarningCropClamped: the computed crop box did not fit within the
	// source image and was translated/shrunk to fit (see FitWithinBounds).
	WarningCropClamped
)

// Warning documents one such automatic decision.
type Warning struct {
	Kind WarningKind
	// FaceCount is set for WarningMultipleFacesDetected.
	FaceCount int
}
