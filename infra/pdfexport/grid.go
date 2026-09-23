package pdfexport

import (
	"fmt"

	"github.com/go-pdf/fpdf"

	"trombi/domain/portrait"
)

const (
	gridPageWidthMM     = 210.0 // A4 portrait
	gridPageHeightMM    = 297.0
	gridMarginMM        = 15.0
	gridGutterMM        = 6.0
	gridCaptionHeightMM = 6.0
	gridCaptionFontPt   = 9.0
)

// GridExporter renders a Batch's exportable portraits as a paginated grid,
// GridLayout.Rows x GridLayout.Cols per page — the trombinoscope sheet
// itself (Docs/SPEC_trombinoscope.md §9.4), as opposed to Exporter, which
// renders one image's detection diagnostic.
type GridExporter struct{}

// Export writes a multi-page PDF to outputPath: one cell per Item in items,
// in order, wrapping to a new row and, once a page is full, a new page.
// Each cell shows the Item's portrait with its Name captioned below.
// Callers are expected to pass only exportable items (see
// portrait.Batch.Exportable) — an Item without a rendered portrait cannot
// be drawn.
func (GridExporter) Export(items []portrait.Item, layout portrait.GridLayout, outputPath string) error {
	if layout.Rows <= 0 || layout.Cols <= 0 {
		return fmt.Errorf("pdfexport: invalid grid layout %+v", layout)
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetFont("Helvetica", "", gridCaptionFontPt)

	contentWidth := gridPageWidthMM - 2*gridMarginMM
	contentHeight := gridPageHeightMM - 2*gridMarginMM
	cellWidth := (contentWidth - float64(layout.Cols-1)*gridGutterMM) / float64(layout.Cols)
	cellHeight := (contentHeight - float64(layout.Rows-1)*gridGutterMM) / float64(layout.Rows)
	imageMaxHeight := cellHeight - gridCaptionHeightMM

	perPage := layout.Rows * layout.Cols

	if len(items) == 0 {
		pdf.AddPage() // an empty batch still produces a valid, blank PDF
	}

	for i, item := range items {
		if i%perPage == 0 {
			pdf.AddPage()
		}
		posInPage := i % perPage
		row := posInPage / layout.Cols
		col := posInPage % layout.Cols

		cellX := gridMarginMM + float64(col)*(cellWidth+gridGutterMM)
		cellY := gridMarginMM + float64(row)*(cellHeight+gridGutterMM)

		if err := drawGridCell(pdf, item, i, cellX, cellY, cellWidth, imageMaxHeight); err != nil {
			return err
		}
	}

	if err := pdf.OutputFileAndClose(outputPath); err != nil {
		return fmt.Errorf("pdfexport: writing %s: %w", outputPath, err)
	}
	return nil
}

// drawGridCell draws one Item's portrait, centered within (maxWidth,
// maxHeight) at (x, y), with its Name captioned below.
func drawGridCell(pdf *fpdf.Fpdf, item portrait.Item, index int, x, y, maxWidth, maxHeight float64) error {
	p := item.Result.Diagnostic.Portrait
	data, err := encodePNG(p.Pixels)
	if err != nil {
		return fmt.Errorf("pdfexport: encoding portrait for %s: %w", item.SourcePath, err)
	}

	w, h := fitWithin(float64(p.Width), float64(p.Height), maxWidth, maxHeight)
	imgX := x + (maxWidth-w)/2
	registerAndDrawImage(pdf, fmt.Sprintf("item-%d", index), "PNG", data, imgX, y, w, h)

	pdf.SetXY(x, y+maxHeight+1)
	pdf.CellFormat(maxWidth, gridCaptionHeightMM-1, item.Name, "", 0, "C", false, 0, "")
	return nil
}
