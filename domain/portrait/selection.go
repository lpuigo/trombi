package portrait

import "errors"

// ErrNoFaceDetected is returned when no face survives detection, per
// Docs/SPEC_trombinoscope.md §8.
var ErrNoFaceDetected = errors.New("portrait: no face detected")

// SelectPrimaryFace picks the face to use for framing, per
// Docs/SPEC_trombinoscope.md §8: the largest by area wins, with a warning
// whenever more than one face was detected. faces is expected to already be
// deduplicated (see FaceDetector) so that overlapping detections of the same
// real face don't trigger a spurious warning.
func SelectPrimaryFace(faces []DetectedFace) (DetectedFace, []Warning, error) {
	if len(faces) == 0 {
		return DetectedFace{}, nil, ErrNoFaceDetected
	}

	largest := faces[0]
	for _, f := range faces[1:] {
		if f.Box.Area() > largest.Box.Area() {
			largest = f
		}
	}

	var warnings []Warning
	if len(faces) > 1 {
		warnings = append(warnings, Warning{Kind: WarningMultipleFacesDetected, FaceCount: len(faces)})
	}
	return largest, warnings, nil
}
