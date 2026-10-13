package service

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"testing"
	"time"

	"trombi/domain/portrait"
)

// Test fixtures: the fake decoder turns the file content into an image
// whose size drives the fake detector.
const (
	photoWithFace = "face"   // 1000x1000 image, one face
	photoNoFace   = "noface" // 1x1 image, no face
	photoCorrupt  = "corrupt"
)

var testFace = portrait.NewBoundingBox(400, 400, 600, 600)

type fakeDecoder struct{}

func (fakeDecoder) Decode(data []byte) (portrait.SourceImage, error) {
	switch string(data) {
	case photoWithFace:
		return portrait.NewSourceImage(image.NewGray(image.Rect(0, 0, 1000, 1000))), nil
	case photoNoFace:
		return portrait.NewSourceImage(image.NewGray(image.Rect(0, 0, 1, 1))), nil
	default:
		return portrait.SourceImage{}, errors.New("corrupt image")
	}
}

// fakeDetector finds testFace on any image large enough to hold it. When
// gate is set, Detect signals started then blocks until gate is closed.
type fakeDetector struct {
	started chan struct{}
	gate    chan struct{}
}

func (d fakeDetector) Detect(src portrait.SourceImage) ([]portrait.DetectedFace, error) {
	if d.gate != nil {
		d.started <- struct{}{}
		<-d.gate
	}
	if src.Width < 1000 {
		return nil, nil
	}
	return []portrait.DetectedFace{{Box: testFace, Score: 10}}, nil
}

type fakeCropper struct{}

func (fakeCropper) Crop(_ portrait.SourceImage, _ portrait.BoundingBox, out portrait.Resolution) (portrait.NormalizedPortrait, error) {
	return portrait.NormalizedPortrait{Pixels: image.NewGray(image.Rect(0, 0, out.Width, out.Height)), Width: out.Width, Height: out.Height}, nil
}

// fakeEncoder writes the encoded image's size and maxDim, for assertions.
type fakeEncoder struct{}

func (fakeEncoder) Encode(w io.Writer, img image.Image, maxDim int) error {
	_, err := fmt.Fprintf(w, "%dx%d/%d", img.Bounds().Dx(), img.Bounds().Dy(), maxDim)
	return err
}

type fakeRenderer struct {
	items  []portrait.Item
	layout portrait.GridLayout
}

func (r *fakeRenderer) Render(_ io.Writer, items []portrait.Item, layout portrait.GridLayout) error {
	r.items, r.layout = items, layout
	return nil
}

func newTestWorkspace(detector fakeDetector) (*Workspace, *fakeRenderer) {
	renderer := &fakeRenderer{}
	deps := WorkspaceDeps{Detector: detector, Cropper: fakeCropper{}, Decoder: fakeDecoder{}, Encoder: fakeEncoder{}, Renderer: renderer}
	ws := NewWorkspace(deps, portrait.DefaultFramingSpec(), portrait.DefaultGridLayout())
	return ws, renderer
}

func mustAdd(t *testing.T, ws *Workspace, path, content string) portrait.Item {
	t.Helper()
	item, err := ws.AddImage(path, []byte(content))
	if err != nil {
		t.Fatalf("AddImage(%q) error = %v", path, err)
	}
	return item
}

func cropBoxUnder(spec portrait.FramingSpec) portrait.BoundingBox {
	return portrait.ComputeCropBox(testFace, spec)
}

func TestWorkspaceAddImage(t *testing.T) {
	ws, _ := newTestWorkspace(fakeDetector{})

	ok := mustAdd(t, ws, "team/alice.jpg", photoWithFace)
	if ok.Result.Status != portrait.StatusSuccess || ok.Name != "alice" {
		t.Errorf("valid photo: Status = %v, Name = %q", ok.Result.Status, ok.Name)
	}
	if ok.Result.SourceImage.Pixels != nil || ok.Result.Diagnostic.Source.Pixels != nil {
		t.Error("decoded source pixels are kept after processing")
	}
	if ok.Result.SourceImage.Width != 1000 {
		t.Errorf("source width = %d, want 1000 (dimensions must survive the release)", ok.Result.SourceImage.Width)
	}

	noFace := mustAdd(t, ws, "bob.jpg", photoNoFace)
	if !errors.Is(noFace.Result.FailureReason, portrait.ErrNoFaceDetected) {
		t.Errorf("photo without face: FailureReason = %v", noFace.Result.FailureReason)
	}
	corrupt := mustAdd(t, ws, "carol.jpg", photoCorrupt)
	if !errors.Is(corrupt.Result.FailureReason, ErrUnreadableImage) {
		t.Errorf("corrupt photo: FailureReason = %v, want ErrUnreadableImage", corrupt.Result.FailureReason)
	}

	if n := len(ws.Snapshot().Batch.Items); n != 3 {
		t.Errorf("Snapshot holds %d items, want 3", n)
	}
}

func TestWorkspaceItemFramingAndReset(t *testing.T) {
	ws, _ := newTestWorkspace(fakeDetector{})
	item := mustAdd(t, ws, "alice.jpg", photoWithFace)
	def := ws.Snapshot().Batch.DefaultSpec

	custom, err := ws.SetItemFraming(item.ID, 0.1, 0.2, 0.15)
	if err != nil {
		t.Fatalf("SetItemFraming() error = %v", err)
	}
	if want := cropBoxUnder(def.WithFraming(0.1, 0.2, 0.15)); custom.Result.Diagnostic.CropBox != want {
		t.Errorf("crop box = %v, want %v", custom.Result.Diagnostic.CropBox, want)
	}
	if !ws.Snapshot().Batch.HasCustomSpec(custom) {
		t.Error("item not reported as custom after SetItemFraming")
	}

	reset, err := ws.ResetItemSpec(item.ID)
	if err != nil {
		t.Fatalf("ResetItemSpec() error = %v", err)
	}
	if reset.Result.Diagnostic.CropBox != item.Result.Diagnostic.CropBox {
		t.Errorf("crop box after reset = %v, want original %v", reset.Result.Diagnostic.CropBox, item.Result.Diagnostic.CropBox)
	}
	if ws.Snapshot().Batch.HasCustomSpec(reset) {
		t.Error("item still reported as custom after ResetItemSpec")
	}
}

func TestWorkspaceRejectsInvalidMargins(t *testing.T) {
	ws, _ := newTestWorkspace(fakeDetector{})
	item := mustAdd(t, ws, "alice.jpg", photoWithFace)

	if _, err := ws.SetItemFraming(item.ID, -0.1, 0.3, 0); !errors.Is(err, portrait.ErrNegativeMargin) {
		t.Errorf("SetItemFraming() error = %v, want ErrNegativeMargin", err)
	}
	if got, _ := ws.Snapshot().Batch.Find(item.ID); got.Spec != item.Spec {
		t.Error("invalid margins altered the item")
	}
}

func TestWorkspaceSetDefaultSpecOverridesCustomItems(t *testing.T) {
	ws, _ := newTestWorkspace(fakeDetector{})
	a := mustAdd(t, ws, "alice.jpg", photoWithFace)
	mustAdd(t, ws, "bob.jpg", photoWithFace)
	mustAdd(t, ws, "carol.jpg", photoNoFace)
	if _, err := ws.SetItemFraming(a.ID, 0.1, 0.1, 0); err != nil {
		t.Fatal(err)
	}

	newDefault := portrait.DefaultFramingSpec().WithFraming(0.5, 0.6, 0)
	if err := ws.SetDefaultSpec(newDefault); err != nil {
		t.Fatalf("SetDefaultSpec() error = %v", err)
	}

	snap := ws.Snapshot()
	for _, it := range snap.Batch.Items {
		if snap.Batch.HasCustomSpec(it) {
			t.Errorf("%s: still custom after SetDefaultSpec", it.Name)
		}
		if it.Result.Status == portrait.StatusSuccess && it.Result.Diagnostic.CropBox != cropBoxUnder(newDefault) {
			t.Errorf("%s: crop box %v not recomputed under the new default", it.Name, it.Result.Diagnostic.CropBox)
		}
	}
	if err := ws.SetDefaultSpec(newDefault.WithFraming(-1, 0, 0)); !errors.Is(err, portrait.ErrNegativeMargin) {
		t.Errorf("SetDefaultSpec(invalid) error = %v, want ErrNegativeMargin", err)
	}
}

func TestWorkspaceDefaultSpecChangedDuringDetection(t *testing.T) {
	detector := fakeDetector{started: make(chan struct{}), gate: make(chan struct{})}
	ws, _ := newTestWorkspace(detector)

	done := make(chan portrait.Item)
	go func() {
		item, _ := ws.AddImage("alice.jpg", []byte(photoWithFace))
		done <- item
	}()
	<-detector.started

	if items := ws.Snapshot().Batch.Items; len(items) != 1 || items[0].Result.Status != portrait.StatusPending {
		t.Fatalf("item not visible as pending during detection: %+v", items)
	}
	newDefault := portrait.DefaultFramingSpec().WithFraming(0.5, 0.6, 0)
	if err := ws.SetDefaultSpec(newDefault); err != nil {
		t.Fatal(err)
	}
	close(detector.gate)

	select {
	case item := <-done:
		if item.Spec != newDefault || item.Result.Diagnostic.CropBox != cropBoxUnder(newDefault) {
			t.Errorf("item processed under a stale spec: Spec = %+v, CropBox = %v", item.Spec, item.Result.Diagnostic.CropBox)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("AddImage did not complete")
	}
}

func TestWorkspaceRemoveAndRename(t *testing.T) {
	ws, _ := newTestWorkspace(fakeDetector{})
	item := mustAdd(t, ws, "alice.jpg", photoWithFace)

	renamed, err := ws.Rename(item.ID, "Alice Martin")
	if err != nil || renamed.Name != "Alice Martin" {
		t.Errorf("Rename() = %q, %v", renamed.Name, err)
	}

	if err := ws.Remove(item.ID); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if len(ws.Snapshot().Batch.Items) != 0 {
		t.Error("item still present after Remove")
	}
	if err := ws.WriteSource(item.ID, 100, io.Discard); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("WriteSource() of removed item error = %v, want ErrItemNotFound", err)
	}
	if err := ws.Remove(item.ID); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("second Remove() error = %v, want ErrItemNotFound", err)
	}
	if _, err := ws.Rename(item.ID, "x"); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("Rename() of removed item error = %v, want ErrItemNotFound", err)
	}
}

func TestWorkspaceWriteImages(t *testing.T) {
	ws, _ := newTestWorkspace(fakeDetector{})
	ok := mustAdd(t, ws, "alice.jpg", photoWithFace)
	noFace := mustAdd(t, ws, "bob.jpg", photoNoFace)

	var buf bytes.Buffer
	if err := ws.WritePortrait(ok.ID, &buf); err != nil || buf.String() != "600x800/0" {
		t.Errorf("WritePortrait() wrote %q, error %v; want the 600x800 portrait", buf.String(), err)
	}
	if err := ws.WritePortrait(noFace.ID, io.Discard); !errors.Is(err, ErrNoPortrait) {
		t.Errorf("WritePortrait() of failed item error = %v, want ErrNoPortrait", err)
	}

	buf.Reset()
	if err := ws.WriteSource(noFace.ID, 1200, &buf); err != nil || buf.String() != "1x1/1200" {
		t.Errorf("WriteSource() wrote %q, error %v; want the decoded source", buf.String(), err)
	}
}

func TestWorkspaceExportPDF(t *testing.T) {
	ws, renderer := newTestWorkspace(fakeDetector{})
	mustAdd(t, ws, "alice.jpg", photoWithFace)
	mustAdd(t, ws, "bob.jpg", photoNoFace)
	layout := portrait.GridLayout{Rows: 2, Cols: 2, HSpacingMM: 3, VSpacingMM: 4}
	if err := ws.SetLayout(layout); err != nil {
		t.Fatal(err)
	}
	if err := ws.SetLayout(portrait.GridLayout{}); !errors.Is(err, portrait.ErrInvalidGridSize) {
		t.Errorf("SetLayout(invalid) error = %v, want ErrInvalidGridSize", err)
	}

	if err := ws.ExportPDF(io.Discard); err != nil {
		t.Fatalf("ExportPDF() error = %v", err)
	}
	if len(renderer.items) != 1 || renderer.items[0].Name != "alice" {
		t.Errorf("rendered items = %+v, want only the exportable one", renderer.items)
	}
	if renderer.layout != layout {
		t.Errorf("rendered layout = %+v, want %+v", renderer.layout, layout)
	}
}
