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
	"strconv"
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
	defaults := portrait.DefaultFramingSpec()

	fs := flag.NewFlagSet("trombi", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		output          string
		topMarginPct    float64
		bottomMarginPct float64
		ratioStr        string
		outWidth        int
		outHeight       int
		help            bool
	)

	registerAlias(fs.StringVar, &output, "o", "output", "", "chemin du PDF de sortie (image unique : PDF de diagnostic ; lot : planche trombinoscope)")
	registerAlias(fs.Float64Var, &topMarginPct, "tm", "topmargin", defaults.TopMargin*100, "marge supérieure, en % de la hauteur du visage")
	registerAlias(fs.Float64Var, &bottomMarginPct, "bm", "bottommargin", defaults.BottomMargin*100, "marge inférieure, en % de la hauteur du visage")
	registerAlias(fs.StringVar, &ratioStr, "ar", "aspectratio", formatRatio(defaults.Ratio), "ratio de sortie largeur:hauteur")
	registerAlias(fs.IntVar, &outWidth, "ow", "outwidth", defaults.Output.Width, "largeur de sortie, en pixels")
	registerAlias(fs.IntVar, &outHeight, "oh", "outheight", defaults.Output.Height, "hauteur de sortie, en pixels")
	registerAlias(fs.BoolVar, &help, "h", "help", false, "affiche cette aide")

	fs.Usage = func() { printUsage(os.Stderr, defaults) }

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if help {
		fs.Usage()
		return 0
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return 2
	}

	ratio, err := parseAspectRatio(ratioStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		return 2
	}

	spec := portrait.FramingSpec{
		TopMargin:    topMarginPct / 100,
		BottomMargin: bottomMarginPct / 100,
		Ratio:        ratio,
		Output:       portrait.Resolution{Width: outWidth, Height: outHeight},
	}
	if err := validateFramingSpec(spec); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		return 2
	}

	paths, err := expandPaths(fs.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		return 2
	}
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "Erreur : aucune image trouvée dans les chemins fournis.")
		return 2
	}

	detector, err := pigodetect.NewDetector()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		return 1
	}

	if len(paths) == 1 {
		return runSingle(detector, spec, paths[0], output)
	}
	return runBatch(detector, spec, paths, output)
}

// runSingle processes exactly one image and renders its single-image
// detection diagnostic PDF (source + both bounding boxes + resulting
// portrait) — see Docs/SPEC_trombinoscope.md §9.4.
func runSingle(detector portrait.FaceDetector, spec portrait.FramingSpec, inputPath, output string) int {
	outputPath := output
	if outputPath == "" {
		outputPath = strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ".pdf"
	}

	src, err := imageio.Load(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		return 1
	}

	result := service.ProcessImage(detector, imageio.Cropper{}, spec, src)

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

// runBatch processes every path in paths, prints a per-file report
// (Docs/SPEC_trombinoscope.md §5/§11), and — when at least one image
// succeeded — renders the trombinoscope grid sheet. A per-file failure never
// stops the rest of the batch (§8); the process exits non-zero only if at
// least one file failed.
func runBatch(detector portrait.FaceDetector, spec portrait.FramingSpec, paths []string, output string) int {
	outputPath := output
	if outputPath == "" {
		outputPath = "trombi.pdf"
	}

	b := service.ProcessBatch(detector, imageio.Cropper{}, imageio.Loader{}, portrait.NewBatch(paths, spec))

	failures := 0
	for _, item := range b.Items {
		switch item.Result.Status {
		case portrait.StatusSuccess:
			fmt.Printf("OK      %s\n", item.SourcePath)
		case portrait.StatusWarning:
			fmt.Printf("OK      %s\n", item.SourcePath)
			for _, w := range item.Result.Warnings {
				fmt.Fprintln(os.Stderr, "Avertissement : "+item.SourcePath+" : "+describeWarning(w))
			}
		case portrait.StatusFailure:
			failures++
			fmt.Fprintln(os.Stderr, "ECHEC   "+item.SourcePath+" : "+describeFailure(item.Result.FailureReason))
		}
	}

	exportable := b.Exportable()
	if len(exportable) == 0 {
		fmt.Fprintln(os.Stderr, "Erreur : aucune image traitée avec succès, aucun PDF généré.")
		return 1
	}

	if err := (pdfexport.GridExporter{}).Export(exportable, portrait.DefaultGridLayout(), outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		return 1
	}

	fmt.Printf("PDF généré : %s (%d/%d portraits)\n", outputPath, len(exportable), len(b.Items))

	if failures > 0 {
		return 1
	}
	return 0
}

// expandPaths turns the CLI's trailing positional arguments into a flat,
// ordered list of image paths: a file argument is kept as-is (even with an
// unsupported extension — letting it reach the loader is what turns it into
// a reported failure per Docs/SPEC_trombinoscope.md §8, instead of being
// silently dropped here), a directory argument is expanded to the supported
// images directly inside it (imageio.ListDir).
func expandPaths(args []string) ([]string, error) {
	var paths []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, fmt.Errorf("chemin invalide %q : %w", arg, err)
		}
		if info.IsDir() {
			dirPaths, err := imageio.ListDir(arg)
			if err != nil {
				return nil, err
			}
			paths = append(paths, dirPaths...)
			continue
		}
		paths = append(paths, arg)
	}
	return paths, nil
}

// registerAlias registers the same flag under a short and a long name, so
// that e.g. -tm and --topmargin both set the same variable. varFn is one of
// flag.FlagSet's StringVar/Float64Var/IntVar/BoolVar methods.
func registerAlias[T any](varFn func(p *T, name string, value T, usage string), p *T, short, long string, value T, usage string) {
	varFn(p, short, value, usage+" (raccourci : -"+short+")")
	varFn(p, long, value, usage)
}

func formatRatio(r portrait.AspectRatio) string {
	return fmt.Sprintf("%g:%g", r.Width, r.Height)
}

func parseAspectRatio(s string) (portrait.AspectRatio, error) {
	invalid := fmt.Errorf("ratio invalide %q (format attendu largeur:hauteur, ex. 3:4)", s)

	w, h, found := strings.Cut(s, ":")
	if !found {
		return portrait.AspectRatio{}, invalid
	}
	width, errW := strconv.ParseFloat(strings.TrimSpace(w), 64)
	height, errH := strconv.ParseFloat(strings.TrimSpace(h), 64)
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return portrait.AspectRatio{}, invalid
	}
	return portrait.AspectRatio{Width: width, Height: height}, nil
}

func validateFramingSpec(spec portrait.FramingSpec) error {
	switch {
	case spec.TopMargin < 0:
		return errors.New("la marge supérieure ne peut pas être négative")
	case spec.BottomMargin < 0:
		return errors.New("la marge inférieure ne peut pas être négative")
	case spec.Output.Width <= 0 || spec.Output.Height <= 0:
		return errors.New("la résolution de sortie doit être strictement positive")
	default:
		return nil
	}
}

func printUsage(w *os.File, defaults portrait.FramingSpec) {
	fmt.Fprintln(w, "Usage: trombi [options] <image|dossier> [image|dossier ...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Détecte le visage sur chaque image, calcule un cadrage de portrait, et génère :")
	fmt.Fprintln(w, "  - avec une seule image en argument : un PDF de diagnostic (image source +")
	fmt.Fprintln(w, "    bounding boxes + portrait recadré) ;")
	fmt.Fprintln(w, "  - avec plusieurs images et/ou un dossier en argument : traitement par lot,")
	fmt.Fprintln(w, "    rapport de traitement par fichier sur la console, et une planche")
	fmt.Fprintln(w, "    trombinoscope au format PDF regroupant les portraits obtenus avec succès.")
	fmt.Fprintln(w, "Un argument dossier est développé en la liste des images qu'il contient")
	fmt.Fprintln(w, "directement (JPEG/PNG) ; un argument fichier est traité tel quel.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options :")
	fmt.Fprintln(w, "  -o,  --output <chemin>    Chemin du PDF de sortie (défaut : même nom que l'image + .pdf en mode image unique, trombi.pdf en mode lot)")
	fmt.Fprintf(w, "  -tm, --topmargin <%%>      Marge supérieure, en %% de la hauteur du visage (défaut %g)\n", defaults.TopMargin*100)
	fmt.Fprintf(w, "  -bm, --bottommargin <%%>   Marge inférieure, en %% de la hauteur du visage (défaut %g)\n", defaults.BottomMargin*100)
	fmt.Fprintf(w, "  -ar, --aspectratio <l:h>  Ratio de sortie largeur:hauteur (défaut %s)\n", formatRatio(defaults.Ratio))
	fmt.Fprintf(w, "  -ow, --outwidth <px>      Largeur de sortie, en pixels (défaut %d)\n", defaults.Output.Width)
	fmt.Fprintf(w, "  -oh, --outheight <px>     Hauteur de sortie, en pixels (défaut %d)\n", defaults.Output.Height)
	fmt.Fprintln(w, "  -h,  --help               Affiche cette aide")
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
