package webserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"trombi/domain/portrait"
	"trombi/service"
	"trombi/usermsg"
)

// badRequestError marks a malformed request, its message being user-facing.
type badRequestError struct{ msg string }

func (e badRequestError) Error() string { return e.msg }

func badRequest(msg string) error { return badRequestError{msg} }

// invalidInputErrors are the domain's validation errors: rejected user
// input, not server failures.
var invalidInputErrors = []error{
	portrait.ErrNegativeMargin,
	portrait.ErrInvalidAspectRatio,
	portrait.ErrInvalidResolution,
	portrait.ErrInvalidGridSize,
	portrait.ErrNegativeSpacing,
	portrait.ErrGridDoesNotFit,
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // the client is gone if this fails
}

// writeError maps err to an HTTP status and a JSON {"error": message} body.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	msg := "erreur interne du serveur."
	var bad badRequestError
	switch {
	case errors.As(err, &bad):
		status, msg = http.StatusBadRequest, bad.msg
	case errors.Is(err, service.ErrItemNotFound):
		status, msg = http.StatusNotFound, "photo introuvable."
	case errors.Is(err, service.ErrNoPortrait):
		status, msg = http.StatusNotFound, "cette photo n'a pas de portrait."
	case isInvalidInput(err):
		status, msg = http.StatusBadRequest, usermsg.Failure(err)
	default:
		s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func isInvalidInput(err error) bool {
	for _, target := range invalidInputErrors {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// decodeJSON reads the request body into v.
func decodeJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return badRequest("requête JSON invalide.")
	}
	return nil
}
