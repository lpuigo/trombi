package portrait

import "errors"

var (
	// ErrInvalidGridSize is returned for a grid with fewer than one row or
	// one column.
	ErrInvalidGridSize = errors.New("portrait: grid must have at least one row and one column")
	// ErrNegativeSpacing is returned for a negative gap between grid cells.
	ErrNegativeSpacing = errors.New("portrait: grid spacing cannot be negative")
)

// GridLayout configures the L (rows) x C (cols) portrait grid produced by
// the batch's final PDF export — the trombinoscope sheet itself, as
// opposed to the single-image diagnostic PDF (see Docs/SPEC_trombinoscope.md
// §9.4). Spacings are in millimetres, the sheet being an A4 page.
type GridLayout struct {
	Rows       int
	Cols       int
	HSpacingMM float64 // gap between two columns
	VSpacingMM float64 // gap between two rows
}

// DefaultGridLayout returns an indicative default: a 4x3 grid, i.e. 12
// portraits per A4 page, 6 mm apart.
func DefaultGridLayout() GridLayout {
	return GridLayout{Rows: 4, Cols: 3, HSpacingMM: 6, VSpacingMM: 6}
}

// Validate checks the layout's own invariants. Whether the cells still fit
// on a page is up to the renderer, which alone knows the page size.
func (l GridLayout) Validate() error {
	switch {
	case l.Rows < 1 || l.Cols < 1:
		return ErrInvalidGridSize
	case l.HSpacingMM < 0 || l.VSpacingMM < 0:
		return ErrNegativeSpacing
	default:
		return nil
	}
}
