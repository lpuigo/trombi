// Command trombi is the CLI entry point: it wires the infra implementations
// into the service layer and turns a portrait.ProcessingResult into
// console output, but contains no business logic of its own.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"trombi/domain/portrait"
	"trombi/infra/facedetect/pigodetect"
	"trombi/infra/imageio"
	"trombi/infra/pdfexport"
	"trombi/service"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("trombi", flag.ContinueOnError)
	output := fs.String("o", "", "Chemin du PDF de sortie (par défaut : même nom que l'image, extension .pdf)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: trombi [-o sortie.pdf] <image.jpg|image.png>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	inputPath := fs.Arg(0)
	outputPath := *output
	if outputPath == "" {
		outputPath = strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ".pdf"
	}

	detector, err := pigodetect.NewDetector()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		return 1
	}

	src, err := imageio.Load(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		return 1
	}

	result := service.ProcessImage(detector, imageio.Cropper{}, portrait.DefaultFramingSpec(), src)

	if result.Status == portrait.StatusFailure {
		fmt.Fprintln(os.Stderr, describeFailure(result.FailureReason))
		return 1
	}

	for _, w := range result.Warnings {
		fmt.Fprintln(os.Stderr, "Avertissement : "+describeWarning(w))
	}

	if err := (pdfexport.Exporter{}).Export(result.Diagnostic, outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		return 1
	}

	fmt.Printf("PDF généré : %s\n", outputPath)
	return 0
}

func describeFailure(err error) string {
	switch {
	case errors.Is(err, portrait.ErrNoFaceDetected):
		return "Erreur : aucun visage détecté dans l'image."
	case errors.Is(err, portrait.ErrFaceExceedsSourceBounds):
		return "Erreur : le visage détecté dépasse les limites de l'image source."
	case errors.Is(err, portrait.ErrCropExcludesFace):
		return "Erreur : impossible de cadrer sans couper une partie du visage détecté."
	default:
		return fmt.Sprintf("Erreur : %v", err)
	}
}

func describeWarning(w portrait.Warning) string {
	switch w.Kind {
	case portrait.WarningMultipleFacesDetected:
		return fmt.Sprintf("%d visages détectés ; le plus grand a été retenu automatiquement.", w.FaceCount)
	case portrait.WarningCropClamped:
		return "le cadrage calculé dépassait les limites de l'image source et a été ajusté automatiquement."
	default:
		return "avertissement non spécifié."
	}
}
