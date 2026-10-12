package portrait

// Batch is a set of Items sharing a default FramingSpec. Individual Items
// may carry their own Spec, overridden by the user in the UI (see
// ReframeItem in package service), until the default is changed again: a
// new default applies to every Item (see SetDefaultSpec).
type Batch struct {
	DefaultSpec FramingSpec
	Items       []Item
	lastID      ItemID
}

// NewBatch builds an unprocessed Batch: one Item per path, all starting out
// with defaultSpec and a StatusPending Result. Processing (detection,
// framing, crop) is the service layer's job — this only assembles the
// batch's shape.
func NewBatch(paths []string, defaultSpec FramingSpec) Batch {
	b := Batch{DefaultSpec: defaultSpec}
	for _, p := range paths {
		b.Add(p)
	}
	return b
}

// Add appends an unprocessed Item for path under the batch's DefaultSpec
// and returns it. IDs are never reused, even after a Remove, so a stale ID
// held by a UI can never designate a different image.
func (b *Batch) Add(path string) Item {
	b.lastID++
	item := NewItem(b.lastID, path, b.DefaultSpec)
	b.Items = append(b.Items, item)
	return item
}

// Find returns the Item with the given id.
func (b Batch) Find(id ItemID) (Item, bool) {
	i := b.indexOf(id)
	if i < 0 {
		return Item{}, false
	}
	return b.Items[i], true
}

// Replace stores item in place of the Item sharing its ID, keeping its
// position in the batch. It reports false if no such Item exists.
func (b *Batch) Replace(item Item) bool {
	i := b.indexOf(item.ID)
	if i < 0 {
		return false
	}
	b.Items[i] = item
	return true
}

// Remove deletes the Item with the given id, preserving the order of the
// others. It reports false if no such Item exists.
func (b *Batch) Remove(id ItemID) bool {
	i := b.indexOf(id)
	if i < 0 {
		return false
	}
	b.Items = append(b.Items[:i:i], b.Items[i+1:]...)
	return true
}

// SetDefaultSpec changes the batch's default framing and applies it to
// every Item, discarding any per-Item override (Docs/SPEC_trombinoscope.md
// §10.2). It only updates the Specs: the Results are now stale and must be
// recomputed by the caller (see service.Workspace).
func (b *Batch) SetDefaultSpec(spec FramingSpec) {
	b.DefaultSpec = spec
	for i := range b.Items {
		b.Items[i].Spec = spec
	}
}

// HasCustomSpec reports whether item's framing was overridden by the user
// rather than following the batch's DefaultSpec.
func (b Batch) HasCustomSpec(item Item) bool {
	return item.Spec != b.DefaultSpec
}

// Exportable returns the Items eligible for the grid export: those whose
// last processing produced a portrait (StatusSuccess or StatusWarning), in
// Batch order.
func (b Batch) Exportable() []Item {
	var out []Item
	for _, it := range b.Items {
		if it.Result.Status == StatusSuccess || it.Result.Status == StatusWarning {
			out = append(out, it)
		}
	}
	return out
}

func (b Batch) indexOf(id ItemID) int {
	for i, it := range b.Items {
		if it.ID == id {
			return i
		}
	}
	return -1
}
