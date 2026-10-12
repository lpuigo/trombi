package webserver

import (
	"bytes"
	"io"
	"net/http"
	"strconv"

	"trombi/domain/portrait"
)

const (
	// maxUploadBytes bounds one uploaded photo; phone pictures are well
	// under it.
	maxUploadBytes = 50 << 20
	// sourcePreviewMaxDim is the largest side of the source image shown in
	// the framing editor — enough for a dialog, far less than a 12 Mpx photo.
	sourcePreviewMaxDim = 1200
)

// addItem receives one photo as multipart/form-data: "file" holds its
// content and the optional "path" its path relative to the folder the user
// picked (the multipart filename alone loses it).
func (s *Server) addItem(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	file, header, err := r.FormFile("file")
	if err != nil {
		s.writeError(w, r, badRequest("fichier manquant ou trop volumineux."))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		s.writeError(w, r, badRequest("lecture du fichier impossible."))
		return
	}

	path := r.FormValue("path")
	if path == "" {
		path = header.Filename
	}
	item, err := s.ws.AddImage(path, data)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.writeItem(w, http.StatusCreated, item)
}

// itemPatch is the body of PATCH /api/items/{id}: every field is optional,
// but margins go in pairs.
type itemPatch struct {
	Name            *string  `json:"name"`
	TopMarginPct    *float64 `json:"topMarginPct"`
	BottomMarginPct *float64 `json:"bottomMarginPct"`
}

func (s *Server) patchItem(w http.ResponseWriter, r *http.Request) {
	id, err := itemID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var p itemPatch
	if err := decodeJSON(r, &p); err != nil {
		s.writeError(w, r, err)
		return
	}
	if (p.TopMarginPct == nil) != (p.BottomMarginPct == nil) {
		s.writeError(w, r, badRequest("les marges haute et basse doivent être fournies ensemble."))
		return
	}

	var item portrait.Item
	if p.Name != nil {
		if item, err = s.ws.Rename(id, *p.Name); err != nil {
			s.writeError(w, r, err)
			return
		}
	}
	if p.TopMarginPct != nil {
		if item, err = s.ws.SetItemMargins(id, *p.TopMarginPct/100, *p.BottomMarginPct/100); err != nil {
			s.writeError(w, r, err)
			return
		}
	}
	if p.Name == nil && p.TopMarginPct == nil {
		s.writeError(w, r, badRequest("aucune modification demandée."))
		return
	}
	s.writeItem(w, http.StatusOK, item)
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, err := itemID(r)
	if err == nil {
		err = s.ws.Remove(id)
	}
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resetItem(w http.ResponseWriter, r *http.Request) {
	id, err := itemID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	item, err := s.ws.ResetItemSpec(id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.writeItem(w, http.StatusOK, item)
}

func (s *Server) getPortrait(w http.ResponseWriter, r *http.Request) {
	s.writeImage(w, r, func(id portrait.ItemID, out io.Writer) error {
		return s.ws.WritePortrait(id, out)
	})
}

func (s *Server) getSource(w http.ResponseWriter, r *http.Request) {
	s.writeImage(w, r, func(id portrait.ItemID, out io.Writer) error {
		return s.ws.WriteSource(id, sourcePreviewMaxDim, out)
	})
}

// writeImage serves the JPEG produced by write for the request's item. The
// image is buffered so that an encoding failure still yields a clean error
// response. Clients put a version key in the query string whenever the
// image changes, so it can be cached.
func (s *Server) writeImage(w http.ResponseWriter, r *http.Request, write func(portrait.ItemID, io.Writer) error) {
	id, err := itemID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var buf bytes.Buffer
	if err := write(id, &buf); err != nil {
		s.writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = buf.WriteTo(w)
}

// writeItem responds with item's view model. Whether its framing is custom
// depends on the batch's current default, hence the snapshot.
func (s *Server) writeItem(w http.ResponseWriter, status int, item portrait.Item) {
	writeJSON(w, status, newItemDTO(s.ws.Snapshot().Batch, item))
}

func itemID(r *http.Request) (portrait.ItemID, error) {
	n, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		return 0, badRequest("identifiant de photo invalide.")
	}
	return portrait.ItemID(n), nil
}
