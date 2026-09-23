package portrait

// Status summarizes the outcome of processing one SourceImage.
type Status int

const (
	// StatusPending is the zero value: no ProcessingResult has been
	// produced yet, e.g. an Item freshly built by NewBatch/NewItem. It
	// exists so an unprocessed Item is never mistaken for a successful one.
	StatusPending Status = iota
	StatusSuccess
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
