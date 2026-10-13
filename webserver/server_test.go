package webserver

import (
	"bytes"
	"encoding/json"
	"image"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"trombi/domain/portrait"
	"trombi/service"
)

// The fakes turn any upload into a 1000x1000 image with one face, except
// content "corrupt", which fails to decode.

type fakeDecoder struct{}

func (fakeDecoder) Decode(data []byte) (portrait.SourceImage, error) {
	if string(data) == "corrupt" {
		return portrait.SourceImage{}, io.ErrUnexpectedEOF
	}
	return portrait.NewSourceImage(image.NewGray(image.Rect(0, 0, 1000, 1000))), nil
}

type fakeDetector struct{}

func (fakeDetector) Detect(portrait.SourceImage) ([]portrait.DetectedFace, error) {
	return []portrait.DetectedFace{{Box: portrait.NewBoundingBox(400, 400, 600, 600)}}, nil
}

type fakeCropper struct{}

func (fakeCropper) Crop(_ portrait.SourceImage, _ portrait.BoundingBox, out portrait.Resolution) (portrait.NormalizedPortrait, error) {
	return portrait.NormalizedPortrait{Pixels: image.NewGray(image.Rect(0, 0, out.Width, out.Height)), Width: out.Width, Height: out.Height}, nil
}

type fakeEncoder struct{}

func (fakeEncoder) Encode(w io.Writer, _ image.Image, _ int) error {
	_, err := w.Write([]byte("jpeg"))
	return err
}

type fakeRenderer struct{}

func (fakeRenderer) Render(w io.Writer, _ []portrait.Item, _ portrait.GridLayout) error {
	_, err := w.Write([]byte("%PDF"))
	return err
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	ws := service.NewWorkspace(service.WorkspaceDeps{
		Detector: fakeDetector{}, Cropper: fakeCropper{}, Decoder: fakeDecoder{}, Encoder: fakeEncoder{}, Renderer: fakeRenderer{},
	}, portrait.DefaultFramingSpec(), portrait.DefaultGridLayout())
	srv := httptest.NewServer(New(ws, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, srv *httptest.Server, method, path, contentType string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func doJSON(t *testing.T, srv *httptest.Server, method, path string, body any, wantStatus int, out any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	res := do(t, srv, method, path, "application/json", r)
	if res.StatusCode != wantStatus {
		msg, _ := io.ReadAll(res.Body)
		t.Fatalf("%s %s: status %d, want %d (%s)", method, path, res.StatusCode, wantStatus, msg)
	}
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
}

func upload(t *testing.T, srv *httptest.Server, path, content string) itemDTO {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "ignored.jpg")
	fw.Write([]byte(content))
	mw.WriteField("path", path)
	mw.Close()

	res := do(t, srv, http.MethodPost, "/api/items", mw.FormDataContentType(), &body)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("upload %s: status %d", path, res.StatusCode)
	}
	var item itemDTO
	if err := json.NewDecoder(res.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	return item
}

func TestAPIItemLifecycle(t *testing.T) {
	srv := newTestServer(t)

	item := upload(t, srv, "team/alice.jpg", "photo")
	if item.Name != "alice" || item.Status != "success" || item.CropBox == nil || item.Custom {
		t.Fatalf("uploaded item = %+v", item)
	}
	failed := upload(t, srv, "bob.jpg", "corrupt")
	if failed.Status != "failure" || len(failed.Messages) != 1 || !strings.Contains(failed.Messages[0], "illisible") {
		t.Errorf("corrupt upload = %+v, want a failure with a French message", failed)
	}

	id := "/api/items/" + itoa(item.ID)
	var patched itemDTO
	doJSON(t, srv, http.MethodPatch, id, map[string]any{"name": "Alice Martin", "topMarginPct": 10, "bottomMarginPct": 20, "horizontalOffsetPct": 15}, http.StatusOK, &patched)
	if patched.Name != "Alice Martin" || !patched.Custom || patched.TopMarginPct != 10 || patched.HorizontalOffsetPct != 15 {
		t.Errorf("patched item = %+v", patched)
	}

	doJSON(t, srv, http.MethodPatch, id, map[string]any{"topMarginPct": -5, "bottomMarginPct": 20, "horizontalOffsetPct": 0}, http.StatusBadRequest, nil)
	doJSON(t, srv, http.MethodPatch, id, map[string]any{"topMarginPct": 5}, http.StatusBadRequest, nil)

	var reset itemDTO
	doJSON(t, srv, http.MethodPost, id+"/reset", nil, http.StatusOK, &reset)
	if reset.Custom {
		t.Error("item still custom after reset")
	}

	if res := do(t, srv, http.MethodGet, id+"/portrait.jpg", "", nil); res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("portrait: status %d, type %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if res := do(t, srv, http.MethodGet, "/api/items/"+itoa(failed.ID)+"/portrait.jpg", "", nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("portrait of failed item: status %d, want 404", res.StatusCode)
	}

	doJSON(t, srv, http.MethodDelete, id, nil, http.StatusNoContent, nil)
	doJSON(t, srv, http.MethodDelete, id, nil, http.StatusNotFound, nil)

	var state stateDTO
	doJSON(t, srv, http.MethodGet, "/api/state", nil, http.StatusOK, &state)
	if len(state.Items) != 1 || state.Items[0].ID != failed.ID {
		t.Errorf("state items = %+v, want only the failed one", state.Items)
	}
}

func TestAPISettings(t *testing.T) {
	srv := newTestServer(t)
	upload(t, srv, "alice.jpg", "photo")

	framing := newFramingDTO(portrait.DefaultFramingSpec().WithFraming(0.5, 0.6, 0))
	var state stateDTO
	doJSON(t, srv, http.MethodPut, "/api/settings/framing", framing, http.StatusOK, &state)
	if state.Framing.TopMarginPct != 50 || state.Items[0].TopMarginPct != 50 {
		t.Errorf("framing not applied: %+v", state)
	}
	framing.Output.Width = 0
	doJSON(t, srv, http.MethodPut, "/api/settings/framing", framing, http.StatusBadRequest, nil)

	var layout layoutDTO
	doJSON(t, srv, http.MethodPut, "/api/settings/layout", layoutDTO{Rows: 2, Cols: 2, HSpacingMM: 3, VSpacingMM: 4}, http.StatusOK, &layout)
	if layout.Rows != 2 || layout.VSpacingMM != 4 {
		t.Errorf("layout = %+v", layout)
	}
	doJSON(t, srv, http.MethodPut, "/api/settings/layout", layoutDTO{}, http.StatusBadRequest, nil)

	res := do(t, srv, http.MethodGet, "/api/export.pdf", "", nil)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/pdf" {
		t.Errorf("export: status %d, type %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
}

func TestLocalOnly(t *testing.T) {
	srv := newTestServer(t)
	cases := []struct {
		name, host, origin string
		want               int
	}{
		{"loopback IP", "", "", http.StatusOK},
		{"localhost name", "localhost:1234", "", http.StatusOK},
		{"same-origin page", "localhost:1234", "http://localhost:1234", http.StatusOK},
		{"DNS rebinding", "evil.example:1234", "", http.StatusForbidden},
		{"cross-site request", "localhost:1234", "http://evil.example", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/state", nil)
			if tc.host != "" {
				req.Host = tc.host
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			res, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != tc.want {
				t.Errorf("status %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}

func TestStaticUI(t *testing.T) {
	srv := newTestServer(t)
	for _, path := range []string{"/", "/app.js", "/style.css"} {
		if res := do(t, srv, http.MethodGet, path, "", nil); res.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status %d", path, res.StatusCode)
		}
	}
}

func itoa(id portrait.ItemID) string { return strconv.Itoa(int(id)) }
