package portrait

import (
	"math"
	"testing"
)

func TestComputeAndFitCropBox(t *testing.T) {
	spec := DefaultFramingSpec()
	targetRatio := spec.Ratio.Value()

	cases := []struct {
		name                      string
		face                      BoundingBox
		sourceWidth, sourceHeight int
		wantErr                   error
	}{
		{
			name:         "comfortably centered face, no clamping needed",
			face:         NewBoundingBox(400, 400, 600, 600),
			sourceWidth:  2000,
			sourceHeight: 2000,
		},
		{
			name:         "large face on a tightly framed source image, as in sample.jpg",
			face:         NewBoundingBox(32, 85, 270, 323),
			sourceWidth:  320,
			sourceHeight: 400,
		},
		{
			name:         "face near the left edge",
			face:         NewBoundingBox(5, 100, 105, 200),
			sourceWidth:  320,
			sourceHeight: 400,
		},
		{
			name:         "face near the top edge",
			face:         NewBoundingBox(100, 2, 200, 102),
			sourceWidth:  320,
			sourceHeight: 400,
		},
		{
			name:         "face bigger than the source image itself",
			face:         NewBoundingBox(-50, -50, 450, 450),
			sourceWidth:  320,
			sourceHeight: 400,
			wantErr:      ErrFaceExceedsSourceBounds,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			crop := ComputeCropBox(tc.face, spec)
			fitted, _, err := FitWithinBounds(crop, tc.face, tc.sourceWidth, tc.sourceHeight)

			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Fatalf("FitWithinBounds() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("FitWithinBounds() unexpected error: %v", err)
			}

			if !fitted.FitsWithin(tc.sourceWidth, tc.sourceHeight) {
				t.Errorf("fitted box %v does not fit within %dx%d", fitted.Rectangle, tc.sourceWidth, tc.sourceHeight)
			}
			if !fitted.Contains(tc.face) {
				t.Errorf("fitted box %v does not contain face box %v", fitted.Rectangle, tc.face.Rectangle)
			}

			gotRatio := float64(fitted.Width()) / float64(fitted.Height())
			if diff := math.Abs(gotRatio - targetRatio); diff > 0.01 {
				t.Errorf("fitted box ratio = %.4f, want ~%.4f (diff %.4f)", gotRatio, targetRatio, diff)
			}
		})
	}
}

func TestComputeCropBoxHorizontalOffset(t *testing.T) {
	face := NewBoundingBox(400, 400, 600, 600) // square, width = height = 200
	spec := DefaultFramingSpec()

	centered := ComputeCropBox(face, spec)
	if got := centered.CenterX(); math.Abs(got-face.CenterX()) > 0.01 {
		t.Errorf("offset=0: crop CenterX = %.2f, want face CenterX %.2f", got, face.CenterX())
	}

	const offset = 0.15 // 15% of face width, to the right
	shifted := ComputeCropBox(face, spec.WithFraming(spec.TopMargin, spec.BottomMargin, offset))

	wantShift := offset * float64(face.Width())
	gotShift := shifted.CenterX() - centered.CenterX()
	if math.Abs(gotShift-wantShift) > 1 { // rounding to the nearest pixel
		t.Errorf("CenterX shift = %.2f, want ~%.2f", gotShift, wantShift)
	}
	// The offset must not change the box's size, only its position.
	if shifted.Width() != centered.Width() || shifted.Height() != centered.Height() {
		t.Errorf("offset changed box size: got %dx%d, want %dx%d",
			shifted.Width(), shifted.Height(), centered.Width(), centered.Height())
	}
}
