package webserver

import (
	"bytes"
	"net/http"
)

func (s *Server) getState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, newStateDTO(s.ws.Snapshot()))
}

// putFraming applies a new default framing to every photo, then responds
// with the whole state since every item may have changed.
func (s *Server) putFraming(w http.ResponseWriter, r *http.Request) {
	var f framingDTO
	if err := decodeJSON(r, &f); err != nil {
		s.writeError(w, r, err)
		return
	}
	if err := s.ws.SetDefaultSpec(f.toSpec()); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.getState(w, r)
}

func (s *Server) putLayout(w http.ResponseWriter, r *http.Request) {
	var l layoutDTO
	if err := decodeJSON(r, &l); err != nil {
		s.writeError(w, r, err)
		return
	}
	if err := s.ws.SetLayout(l.toLayout()); err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newLayoutDTO(s.ws.Snapshot().Layout))
}

// exportPDF serves the trombinoscope sheet as a download. It is buffered
// so that a rendering failure (e.g. a layout that does not fit on a page)
// still yields a clean error response.
func (s *Server) exportPDF(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if err := s.ws.ExportPDF(&buf); err != nil {
		s.writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="trombinoscope.pdf"`)
	_, _ = buf.WriteTo(w)
}
