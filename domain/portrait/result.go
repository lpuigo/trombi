package portrait

// Status summarizes the outcome of processing one SourceImage.
type Status int

const (
	StatusSuccess Status = iota
	StatusWarning
	StatusFailure
)

// ProcessingResult is the outcome of running the portrait pipeline on one
// SourceImage — the aggregate consumed by exporters and, in a future
// version, by the batch report.
type ProcessingResult struct {
	Status        Status
	SourceImage   SourceImage
	DetectedFaces []DetectedFace
	SelectedFace  DetectedFace
	Diagnostic    Diagnostic
	Warnings      []Warning
	FailureReason error
}
