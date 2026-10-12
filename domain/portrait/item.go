package portrait

import (
	"path/filepath"
	"strings"
)

// ItemID identifies an Item within its Batch. It is needed because
// SourcePath alone is not unique once images come from a browser upload:
// two folders can each hold an "alice.jpg".
type ItemID int

// Item is one source image tracked through a Batch: its file path, a
// user-facing Name (used as the caption under its portrait in the grid
// export), the framing parameters applied to it, and the outcome of running
// the pipeline with those parameters. Keeping Name and Spec alongside
// Result — rather than just the Result — is what lets a UI rename an image
// or edit its framing and reprocess only that Item, without touching the
// rest of the Batch.
type Item struct {
	ID         ItemID
	SourcePath string
	Name       string
	Spec       FramingSpec
	Result     ProcessingResult
}

// NewItem builds an unprocessed Item for path under spec, with Name
// defaulting to the file's base name without its extension — editable
// afterwards by the user.
func NewItem(id ItemID, path string, spec FramingSpec) Item {
	return Item{ID: id, SourcePath: path, Name: defaultItemName(path), Spec: spec}
}

func defaultItemName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
