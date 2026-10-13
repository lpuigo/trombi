package service

import (
	"errors"
	"fmt"
	"image"
	"io"
	"runtime"
	"slices"
	"sync"

	"trombi/domain/portrait"
)

var (
	// ErrItemNotFound is returned when an operation targets an ItemID that
	// is not (or no longer) part of the Workspace.
	ErrItemNotFound = errors.New("service: item not found")
	// ErrNoPortrait is returned when asking for the portrait of an Item
	// that has none (still pending, or failed).
	ErrNoPortrait = errors.New("service: item has no portrait")
	// ErrUnreadableImage wraps a decoding failure: the file is corrupt or
	// not in a supported format (Docs/SPEC_trombinoscope.md §8).
	ErrUnreadableImage = errors.New("service: unreadable or unsupported image")
)

// ImageDecoder decodes an in-memory image file. The web GUI receives image
// bytes from the browser rather than paths on disk, hence this counterpart
// to ImageLoader.
type ImageDecoder interface {
	Decode(data []byte) (portrait.SourceImage, error)
}

// ImageEncoder writes an image for display, downscaled to fit within
// maxDim pixels on either side when maxDim > 0.
type ImageEncoder interface {
	Encode(w io.Writer, img image.Image, maxDim int) error
}

// GridRenderer writes the trombinoscope sheet for items laid out per layout.
type GridRenderer interface {
	Render(w io.Writer, items []portrait.Item, layout portrait.GridLayout) error
}

// WorkspaceDeps groups the implementations a Workspace delegates to.
type WorkspaceDeps struct {
	Detector portrait.FaceDetector
	Cropper  Cropper
	Decoder  ImageDecoder
	Encoder  ImageEncoder
	Renderer GridRenderer
}

// Snapshot is a consistent, caller-owned copy of a Workspace's state.
type Snapshot struct {
	Batch  portrait.Batch
	Layout portrait.GridLayout
}

// Workspace is the single trombinoscope edited through the web GUI: a
// Batch, its grid layout, and the original bytes of every image. It is safe
// for concurrent use by HTTP handlers.
//
// Decoded source images are not kept once an Item is processed: at about
// 48 MB per 12 Mpx photo, a few hundred of them would not fit in memory.
// The original file bytes are kept instead and decoded again whenever an
// Item has to be reframed.
type Workspace struct {
	detector portrait.FaceDetector
	cropper  Cropper
	decoder  ImageDecoder
	encoder  ImageEncoder
	renderer GridRenderer

	mu      sync.Mutex
	batch   portrait.Batch
	layout  portrait.GridLayout
	sources map[portrait.ItemID][]byte
}

// NewWorkspace returns an empty Workspace starting from spec and layout,
// both assumed already validated.
func NewWorkspace(deps WorkspaceDeps, spec portrait.FramingSpec, layout portrait.GridLayout) *Workspace {
	return &Workspace{
		detector: deps.Detector,
		cropper:  deps.Cropper,
		decoder:  deps.Decoder,
		encoder:  deps.Encoder,
		renderer: deps.Renderer,
		batch:    portrait.NewBatch(nil, spec),
		layout:   layout,
		sources:  make(map[portrait.ItemID][]byte),
	}
}

// Snapshot returns a copy of the current state.
func (w *Workspace) Snapshot() Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	b := w.batch
	b.Items = slices.Clone(b.Items)
	return Snapshot{Batch: b, Layout: w.layout}
}

// AddImage adds the image file data, named path, and runs the full
// pipeline on it. The Item is visible as StatusPending in Snapshots while
// detection runs, and detection runs without holding the lock so several
// uploads can be processed concurrently. A decoding or detection failure is
// recorded in the returned Item's Result, never returned as an error;
// ErrItemNotFound means the Item was removed before processing completed.
func (w *Workspace) AddImage(path string, data []byte) (portrait.Item, error) {
	w.mu.Lock()
	pending := w.batch.Add(path)
	w.sources[pending.ID] = data
	w.mu.Unlock()

	var result portrait.ProcessingResult
	src, err := w.decoder.Decode(data)
	if err != nil {
		result = portrait.ProcessingResult{Status: portrait.StatusFailure, FailureReason: fmt.Errorf("%w: %v", ErrUnreadableImage, err)}
	} else {
		result = ProcessImage(w.detector, w.cropper, pending.Spec, src)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	item, ok := w.batch.Find(pending.ID)
	if !ok {
		return portrait.Item{}, ErrItemNotFound
	}
	item.Result = result
	if item.Spec != pending.Spec {
		// The default framing changed while detection was running: the
		// Result must match the Spec now recorded on the Item.
		item, _ = ReframeItem(w.cropper, item.Spec, item)
	}
	item.Result = releaseSourcePixels(item.Result)
	w.batch.Replace(item)
	return item, nil
}

// Remove deletes the Item with the given id.
func (w *Workspace) Remove(id portrait.ItemID) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.batch.Remove(id) {
		return ErrItemNotFound
	}
	delete(w.sources, id)
	return nil
}

// Rename changes the caption of the Item with the given id.
func (w *Workspace) Rename(id portrait.ItemID, name string) (portrait.Item, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	item, ok := w.batch.Find(id)
	if !ok {
		return portrait.Item{}, ErrItemNotFound
	}
	item.Name = name
	w.batch.Replace(item)
	return item, nil
}

// SetItemFraming overrides the margins and horizontal offset of one Item
// and reframes it.
func (w *Workspace) SetItemFraming(id portrait.ItemID, top, bottom, horizontalOffset float64) (portrait.Item, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	spec := w.batch.DefaultSpec.WithFraming(top, bottom, horizontalOffset)
	if err := spec.Validate(); err != nil {
		return portrait.Item{}, err
	}
	return w.reframeByID(id, spec)
}

// ResetItemSpec makes one Item follow the default framing again.
func (w *Workspace) ResetItemSpec(id portrait.ItemID) (portrait.Item, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.reframeByID(id, w.batch.DefaultSpec)
}

// SetDefaultSpec changes the default framing and applies it to every Item,
// overriding per-Item margins (Docs/SPEC_trombinoscope.md §10.2). Items are
// reframed in parallel, since each one has to be decoded again.
func (w *Workspace) SetDefaultSpec(spec portrait.FramingSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	items := w.batch.Items
	var wg sync.WaitGroup
	slots := make(chan struct{}, runtime.NumCPU())
	for i := range items {
		if items[i].Spec == spec {
			continue // already framed under spec
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			items[i] = w.reframe(items[i], spec)
		}()
	}
	wg.Wait()

	w.batch.SetDefaultSpec(spec)
	return nil
}

// SetLayout changes the grid layout of the exported sheet.
func (w *Workspace) SetLayout(layout portrait.GridLayout) error {
	if err := layout.Validate(); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.layout = layout
	return nil
}

// WritePortrait writes the normalized portrait of the Item with the given
// id, at its output resolution.
func (w *Workspace) WritePortrait(id portrait.ItemID, out io.Writer) error {
	w.mu.Lock()
	item, ok := w.batch.Find(id)
	w.mu.Unlock()
	if !ok {
		return ErrItemNotFound
	}
	pixels := item.Result.Diagnostic.Portrait.Pixels
	if pixels == nil {
		return ErrNoPortrait
	}
	return w.encoder.Encode(out, pixels, 0)
}

// WriteSource writes the original image of the Item with the given id,
// orientation-corrected and downscaled to fit within maxDim, e.g. for the
// framing editor to display it under its face and crop boxes (which stay
// in full-resolution coordinates: the caller rescales them).
func (w *Workspace) WriteSource(id portrait.ItemID, maxDim int, out io.Writer) error {
	w.mu.Lock()
	data, ok := w.sources[id]
	w.mu.Unlock()
	if !ok {
		return ErrItemNotFound
	}
	src, err := w.decoder.Decode(data)
	if err != nil {
		return err
	}
	return w.encoder.Encode(out, src.Pixels, maxDim)
}

// ExportPDF renders the trombinoscope sheet for every exportable Item.
func (w *Workspace) ExportPDF(out io.Writer) error {
	w.mu.Lock()
	items := w.batch.Exportable()
	layout := w.layout
	w.mu.Unlock()
	return w.renderer.Render(out, items, layout)
}

// reframeByID reframes the Item with the given id under spec. Callers must
// hold w.mu.
func (w *Workspace) reframeByID(id portrait.ItemID, spec portrait.FramingSpec) (portrait.Item, error) {
	item, ok := w.batch.Find(id)
	if !ok {
		return portrait.Item{}, ErrItemNotFound
	}
	item = w.reframe(item, spec)
	w.batch.Replace(item)
	return item, nil
}

// reframe returns item framed under spec. An Item without a detected face
// (failed, or still pending detection) only records the new spec: there is
// nothing to reframe yet.
func (w *Workspace) reframe(item portrait.Item, spec portrait.FramingSpec) portrait.Item {
	item.Spec = spec
	if len(item.Result.DetectedFaces) == 0 {
		return item
	}
	src, err := w.decoder.Decode(w.sources[item.ID])
	if err != nil {
		item.Result = portrait.ProcessingResult{Status: portrait.StatusFailure, FailureReason: err}
		return item
	}
	item.Result.SourceImage = src
	item, _ = ReframeItem(w.cropper, spec, item) // cannot fail: item has detected faces
	item.Result = releaseSourcePixels(item.Result)
	return item
}

// releaseSourcePixels drops the decoded source image from r, keeping its
// dimensions (see the Workspace doc comment for why).
func releaseSourcePixels(r portrait.ProcessingResult) portrait.ProcessingResult {
	r.SourceImage.Pixels = nil
	r.Diagnostic.Source.Pixels = nil
	return r
}
