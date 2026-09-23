package portrait

import (
	"path/filepath"
	"strings"
)

// Item is one source image tracked through a Batch: its file path, a
// user-facing Name (used as the caption under its portrait in the grid
// export), the framing parameters applied to it, and the outcome of running
// the pipeline with those parameters. Keeping Name and Spec alongside
// Result — rather than just the Result — is what lets a UI rename an image
// or edit its framing and reprocess only that Item, without touching the
// rest of the Batch.
type Item struct {
	SourcePath string
	Name       string
	Spec       FramingSpec
	Result     ProcessingResult
}

// NewItem builds an unprocessed Item for path under spec, with Name
// defaulting to the file's base name without its extension — editable
// afterwards by the user.
func NewItem(path string, spec FramingSpec) Item {
	return Item{SourcePath: path, Name: defaultItemName(path), Spec: spec}
}

func defaultItemName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
