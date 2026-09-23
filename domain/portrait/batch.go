package portrait

// Batch is a set of Items sharing a default FramingSpec, built from every
// supported image file in a source directory (see NewBatch). Individual
// Items may later carry their own Spec, overridden by the user in the UI
// (see ReframeItem in package service).
type Batch struct {
	DefaultSpec FramingSpec
	Items       []Item
}

// NewBatch builds an unprocessed Batch: one Item per path, all starting out
// with defaultSpec and a StatusPending Result. Processing (detection,
// framing, crop) is the service layer's job — this only assembles the
// batch's shape.
func NewBatch(paths []string, defaultSpec FramingSpec) Batch {
	items := make([]Item, len(paths))
	for i, p := range paths {
		items[i] = NewItem(p, defaultSpec)
	}
	return Batch{DefaultSpec: defaultSpec, Items: items}
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
