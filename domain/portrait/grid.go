package portrait

// GridLayout configures the L (rows) x C (cols) portrait grid produced by
// the batch's final PDF export — the trombinoscope sheet itself, as
// opposed to the single-image diagnostic PDF (see Docs/SPEC_trombinoscope.md
// §9.4).
type GridLayout struct {
	Rows int
	Cols int
}

// DefaultGridLayout returns an indicative default: a 4x3 grid, i.e. 12
// portraits per A4 page.
func DefaultGridLayout() GridLayout {
	return GridLayout{Rows: 4, Cols: 3}
}
