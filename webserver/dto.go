package webserver

import (
	"math"

	"trombi/domain/portrait"
	"trombi/service"
	"trombi/usermsg"
)

// The JSON view models below are the API's contract with static/app.js.
// Margins are exchanged in percent, as the user sees them.

type ratioDTO struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type sizeDTO struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type boxDTO struct {
	X0 int `json:"x0"`
	Y0 int `json:"y0"`
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
}

// derivedDTO carries the display-only values derived from a FramingSpec
// (Docs/SPEC_trombinoscope.md §9.2).
type derivedDTO struct {
	HorizontalMarginPct float64 `json:"horizontalMarginPct"`
	FacePositionPct     float64 `json:"facePositionPct"`
}

type framingDTO struct {
	TopMarginPct    float64    `json:"topMarginPct"`
	BottomMarginPct float64    `json:"bottomMarginPct"`
	Ratio           ratioDTO   `json:"ratio"`
	Output          sizeDTO    `json:"output"`
	Derived         derivedDTO `json:"derived"` // ignored on input
}

type layoutDTO struct {
	Rows       int     `json:"rows"`
	Cols       int     `json:"cols"`
	HSpacingMM float64 `json:"hSpacingMm"`
	VSpacingMM float64 `json:"vSpacingMm"`
}

type itemDTO struct {
	ID              portrait.ItemID `json:"id"`
	Name            string          `json:"name"`
	SourcePath      string          `json:"sourcePath"`
	Status          string          `json:"status"`
	Custom          bool            `json:"custom"`
	TopMarginPct    float64         `json:"topMarginPct"`
	BottomMarginPct float64         `json:"bottomMarginPct"`
	Derived         derivedDTO      `json:"derived"`
	Messages        []string        `json:"messages"`
	Source          *sizeDTO        `json:"source,omitempty"`
	FaceBox         *boxDTO         `json:"faceBox,omitempty"`
	CropBox         *boxDTO         `json:"cropBox,omitempty"`
}

type stateDTO struct {
	Framing framingDTO `json:"framing"`
	Layout  layoutDTO  `json:"layout"`
	Items   []itemDTO  `json:"items"`
}

func toPct(fraction float64) float64 { return math.Round(fraction*10000) / 100 }

func newDerivedDTO(spec portrait.FramingSpec) derivedDTO {
	return derivedDTO{
		HorizontalMarginPct: toPct(spec.HorizontalMargin()),
		FacePositionPct:     toPct(spec.FaceVerticalPosition()),
	}
}

func newFramingDTO(spec portrait.FramingSpec) framingDTO {
	return framingDTO{
		TopMarginPct:    toPct(spec.TopMargin),
		BottomMarginPct: toPct(spec.BottomMargin),
		Ratio:           ratioDTO{Width: spec.Ratio.Width, Height: spec.Ratio.Height},
		Output:          sizeDTO{Width: spec.Output.Width, Height: spec.Output.Height},
		Derived:         newDerivedDTO(spec),
	}
}

func (f framingDTO) toSpec() portrait.FramingSpec {
	return portrait.FramingSpec{
		TopMargin:    f.TopMarginPct / 100,
		BottomMargin: f.BottomMarginPct / 100,
		Ratio:        portrait.AspectRatio{Width: f.Ratio.Width, Height: f.Ratio.Height},
		Output:       portrait.Resolution{Width: f.Output.Width, Height: f.Output.Height},
	}
}

func newLayoutDTO(l portrait.GridLayout) layoutDTO {
	return layoutDTO{Rows: l.Rows, Cols: l.Cols, HSpacingMM: l.HSpacingMM, VSpacingMM: l.VSpacingMM}
}

func (l layoutDTO) toLayout() portrait.GridLayout {
	return portrait.GridLayout{Rows: l.Rows, Cols: l.Cols, HSpacingMM: l.HSpacingMM, VSpacingMM: l.VSpacingMM}
}

var statusNames = map[portrait.Status]string{
	portrait.StatusPending: "pending",
	portrait.StatusSuccess: "success",
	portrait.StatusWarning: "warning",
	portrait.StatusFailure: "failure",
}

func newBoxDTO(b portrait.BoundingBox) *boxDTO {
	return &boxDTO{X0: b.Min.X, Y0: b.Min.Y, X1: b.Max.X, Y1: b.Max.Y}
}

func newItemDTO(batch portrait.Batch, it portrait.Item) itemDTO {
	r := it.Result
	dto := itemDTO{
		ID:              it.ID,
		Name:            it.Name,
		SourcePath:      it.SourcePath,
		Status:          statusNames[r.Status],
		Custom:          batch.HasCustomSpec(it),
		TopMarginPct:    toPct(it.Spec.TopMargin),
		BottomMarginPct: toPct(it.Spec.BottomMargin),
		Derived:         newDerivedDTO(it.Spec),
		Messages:        []string{},
	}
	if r.FailureReason != nil {
		dto.Messages = append(dto.Messages, usermsg.Failure(r.FailureReason))
	}
	for _, w := range r.Warnings {
		dto.Messages = append(dto.Messages, usermsg.Warning(w))
	}
	if r.SourceImage.Width > 0 {
		dto.Source = &sizeDTO{Width: r.SourceImage.Width, Height: r.SourceImage.Height}
	}
	if len(r.DetectedFaces) > 0 {
		dto.FaceBox = newBoxDTO(r.SelectedFace.Box)
	}
	if r.Diagnostic.Portrait.Pixels != nil {
		dto.CropBox = newBoxDTO(r.Diagnostic.CropBox)
	}
	return dto
}

func newStateDTO(snap service.Snapshot) stateDTO {
	items := make([]itemDTO, len(snap.Batch.Items))
	for i, it := range snap.Batch.Items {
		items[i] = newItemDTO(snap.Batch, it)
	}
	return stateDTO{
		Framing: newFramingDTO(snap.Batch.DefaultSpec),
		Layout:  newLayoutDTO(snap.Layout),
		Items:   items,
	}
}
