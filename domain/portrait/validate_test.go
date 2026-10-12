package portrait

import (
	"errors"
	"testing"
)

func TestFramingSpecValidate(t *testing.T) {
	valid := DefaultFramingSpec()
	cases := []struct {
		name    string
		spec    FramingSpec
		wantErr error
	}{
		{"default spec", valid, nil},
		{"zero margins", valid.WithMargins(0, 0), nil},
		{"negative top margin", valid.WithMargins(-0.1, 0.3), ErrNegativeMargin},
		{"negative bottom margin", valid.WithMargins(0.3, -0.1), ErrNegativeMargin},
		{"zero ratio term", FramingSpec{Ratio: AspectRatio{Width: 0, Height: 4}, Output: valid.Output}, ErrInvalidAspectRatio},
		{"zero resolution", FramingSpec{Ratio: valid.Ratio, Output: Resolution{Width: 600}}, ErrInvalidResolution},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.spec.Validate(); !errors.Is(err, tc.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestGridLayoutValidate(t *testing.T) {
	cases := []struct {
		name    string
		layout  GridLayout
		wantErr error
	}{
		{"default layout", DefaultGridLayout(), nil},
		{"no spacing", GridLayout{Rows: 1, Cols: 1}, nil},
		{"no rows", GridLayout{Rows: 0, Cols: 3}, ErrInvalidGridSize},
		{"no cols", GridLayout{Rows: 4, Cols: 0}, ErrInvalidGridSize},
		{"negative spacing", GridLayout{Rows: 4, Cols: 3, HSpacingMM: -1}, ErrNegativeSpacing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.layout.Validate(); !errors.Is(err, tc.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
