package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"trombi/domain/portrait"
	"trombi/infra/facedetect/pigodetect"
	"trombi/infra/imageio"
	"trombi/infra/pdfexport"
	"trombi/service"
	"trombi/webserver"
)

// previewJPEGQuality is used for the images displayed in the browser, not
// for exported files.
const previewJPEGQuality = 85

// runServe implements "trombi serve": it wires a service.Workspace into the
// web GUI and serves it on the loopback interface only, so that no photo
// is ever reachable from another machine (Docs/SPEC_trombinoscope.md §11).
func runServe(args []string) int {
	fs := flag.NewFlagSet("trombi serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	port := fs.Int("port", 0, "port d'écoute sur 127.0.0.1 (0 : choisi automatiquement)")
	noBrowser := fs.Bool("no-browser", false, "ne pas ouvrir le navigateur au démarrage")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	detector, err := pigodetect.NewDetector()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		return 1
	}
	ws := service.NewWorkspace(service.WorkspaceDeps{
		Detector: detector,
		Cropper:  imageio.Cropper{},
		Decoder:  imageio.Decoder{},
		Encoder:  imageio.JPEGEncoder{Quality: previewJPEGQuality},
		Renderer: pdfexport.GridExporter{},
	}, portrait.DefaultFramingSpec(), portrait.DefaultGridLayout())

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		return 1
	}
	url := fmt.Sprintf("http://localhost:%d/", listener.Addr().(*net.TCPAddr).Port)
	fmt.Printf("trombi est disponible sur %s (Ctrl+C pour arrêter)\n", url)

	if !*noBrowser {
		if err := openBrowser(url); err != nil {
			fmt.Fprintf(os.Stderr, "Ouverture du navigateur impossible (%v) : ouvrez %s manuellement.\n", err, url)
		}
	}

	server := &http.Server{
		Handler:           webserver.New(ws, log),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := server.Serve(listener); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		return 1
	}
	return 0
}
