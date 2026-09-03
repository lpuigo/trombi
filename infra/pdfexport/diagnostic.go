// Package pdfexport renders a portrait.Diagnostic to a one-page PDF: the
// source image with both bounding boxes overlaid, and the resulting
// portrait below (see Docs/SPEC_trombinoscope.md §9.4).
package pdfexport

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"

	"github.com/go-pdf/fpdf"

	"trombi/domain/portrait"
)

const (
	pageWidthMM  = 210.0 // A4 portrait
	pageHeightMM = 297.0
	marginMM     = 15.0
	gapMM        = 10.0

	boxLineWidthMM = 0.6
	// sourceHeightShare is how much of the vertical space between margins is
	// given to the source image, the rest going to the resulting portrait.
	sourceHeightShare = 0.6
)

// faceBoxColor and cropBoxColor are the two overlay colors requested for the
// diagnostic: the native face bounding box and the crop bounding box.
var (
	faceBoxColor = [3]int{220, 30, 30} // red
	cropBoxColor = [3]int{30, 150, 60} // green
)

// Exporter renders a portrait.Diagnostic to a PDF file.
type Exporter struct{}

// Export writes a one-page A4 PDF to outputPath.
func (Exporter) Export(d portrait.Diagnostic, outputPath string) error {
	sourceBytes, err := encodePNG(d.Source.Pixels)
	if err != nil {
		return fmt.Errorf("pdfexport: encoding source image: %w", err)
	}
	portraitBytes, err := encodePNG(d.Portrait.Pixels)
	if err != nil {
		return fmt.Errorf("pdfexport: encoding portrait image: %w", err)
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	contentWidth := pageWidthMM - 2*marginMM
	availableHeight := pageHeightMM - 2*marginMM - gapMM
	sourceMaxHeight := availableHeight * sourceHeightShare
	portraitMaxHeight := availableHeight - sourceMaxHeight

	sourceW, sourceH := fitWithin(float64(d.Source.Width), float64(d.Source.Height), contentWidth, sourceMaxHeight)
	sourceX := marginMM + (contentWidth-sourceW)/2
	sourceY := marginMM

	registerAndDrawImage(pdf, "source", "PNG", sourceBytes, sourceX, sourceY, sourceW, sourceH)

	scale := sourceW / float64(d.Source.Width)
	drawBox(pdf, d.FaceBox, sourceX, sourceY, scale, faceBoxColor)
	drawBox(pdf, d.CropBox, sourceX, sourceY, scale, cropBoxColor)

	portraitW, portraitH := fitWithin(float64(d.Portrait.Width), float64(d.Portrait.Height), contentWidth, portraitMaxHeight)
	portraitX := marginMM + (contentWidth-portraitW)/2
	portraitY := sourceY + sourceH + gapMM

	registerAndDrawImage(pdf, "portrait", "PNG", portraitBytes, portraitX, portraitY, portraitW, portraitH)

	if err := pdf.OutputFileAndClose(outputPath); err != nil {
		return fmt.Errorf("pdfexport: writing %s: %w", outputPath, err)
	}
	return nil
}

// drawBox draws box's outline, translated by the (originX, originY) mm
// offset of the displayed source image and scaled from source-pixel space
// to mm by scale.
func drawBox(pdf *fpdf.Fpdf, box portrait.BoundingBox, originX, originY, scale float64, color [3]int) {
	x := originX + float64(box.Min.X)*scale
	y := originY + float64(box.Min.Y)*scale
	w := float64(box.Width()) * scale
	h := float64(box.Height()) * scale

	pdf.SetDrawColor(color[0], color[1], color[2])
	pdf.SetLineWidth(boxLineWidthMM)
	pdf.Rect(x, y, w, h, "D")
}

func registerAndDrawImage(pdf *fpdf.Fpdf, name, imageType string, data []byte, x, y, w, h float64) {
	opts := fpdf.ImageOptions{ImageType: imageType}
	pdf.RegisterImageOptionsReader(name, opts, bytes.NewReader(data))
	pdf.ImageOptions(name, x, y, w, h, false, opts, 0, "")
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fitWithin scales (w, h) down to fit within (maxW, maxH), preserving
// aspect ratio, without ever upscaling.
func fitWithin(w, h, maxW, maxH float64) (float64, float64) {
	scale := math.Min(maxW/w, maxH/h)
	if scale > 1 {
		scale = 1
	}
	return w * scale, h * scale
}
