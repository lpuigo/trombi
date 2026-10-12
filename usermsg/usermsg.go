// Package usermsg turns domain errors and warnings into user-facing French
// messages. It is shared by the user interfaces (CLI, web GUI) so that
// neither has to duplicate the wording, and the domain stays free of
// presentation concerns (see portrait.WarningKind).
package usermsg

import (
	"errors"
	"fmt"

	"trombi/domain/portrait"
	"trombi/service"
)

// Failure describes why an image or a user input was rejected.
func Failure(err error) string {
	switch {
	case errors.Is(err, portrait.ErrNoFaceDetected):
		return "aucun visage détecté dans l'image."
	case errors.Is(err, portrait.ErrFaceExceedsSourceBounds):
		return "le visage détecté dépasse les limites de l'image source."
	case errors.Is(err, portrait.ErrCropExcludesFace):
		return "impossible de cadrer sans couper une partie du visage détecté."
	case errors.Is(err, portrait.ErrNegativeMargin):
		return "les marges ne peuvent pas être négatives."
	case errors.Is(err, portrait.ErrInvalidAspectRatio):
		return "le ratio de sortie doit être strictement positif."
	case errors.Is(err, portrait.ErrInvalidResolution):
		return "la résolution de sortie doit être strictement positive."
	case errors.Is(err, portrait.ErrInvalidGridSize):
		return "la grille doit avoir au moins une ligne et une colonne."
	case errors.Is(err, portrait.ErrNegativeSpacing):
		return "les espacements ne peuvent pas être négatifs."
	case errors.Is(err, portrait.ErrGridDoesNotFit):
		return "la grille ne tient pas sur une page A4 : réduisez le nombre de photos ou les espacements."
	case errors.Is(err, service.ErrUnreadableImage):
		return "image illisible ou format non supporté (JPEG et PNG uniquement)."
	default:
		return fmt.Sprintf("%v", err)
	}
}

// Warning describes an automatic decision taken while processing an image.
func Warning(w portrait.Warning) string {
	switch w.Kind {
	case portrait.WarningMultipleFacesDetected:
		return fmt.Sprintf("%d visages détectés ; le plus grand a été retenu automatiquement.", w.FaceCount)
	case portrait.WarningCropClamped:
		return "le cadrage calculé dépassait les limites de l'image source et a été ajusté automatiquement."
	default:
		return "avertissement non spécifié."
	}
}
