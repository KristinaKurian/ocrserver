package tesseract

import (
	"context"

	"github.com/KristinaKurian/ocrserver/internal/ocrmodel"
	"github.com/otiai10/gosseract/v2"
)

type Engine struct {
	client           *gosseract.Client
	currentLanguages []string
}

func New() (ocrmodel.Engine, error) {
	client := gosseract.NewClient()
	client.Trim = false

	return &Engine{
		client:           client,
		currentLanguages: []string{"eng"},
	}, nil
}

func (e *Engine) Recognize(ctx context.Context, request ocrmodel.Request) (ocrmodel.Result, error) {
	if err := ctx.Err(); err != nil {
		return ocrmodel.Result{}, err
	}

	// gosseract must be re-initialized when the language changes. Assigning
	// client.Languages directly does not trigger re-initialization.
	if !sameStrings(e.currentLanguages, request.Languages) {
		if err := e.client.SetLanguage(request.Languages...); err != nil {
			return ocrmodel.Result{}, err
		}
		e.currentLanguages = append(e.currentLanguages[:0], request.Languages...)
	}

	// Always set the whitelist, including an empty value, so a whitelist from
	// the previous job cannot leak into the next job handled by this worker.
	if err := e.client.SetWhitelist(request.Whitelist); err != nil {
		return ocrmodel.Result{}, err
	}

	if err := e.client.SetImageFromBytes(request.Image); err != nil {
		return ocrmodel.Result{}, err
	}

	var (
		text string
		err  error
	)

	switch request.Format {
	case ocrmodel.FormatHOCR:
		text, err = e.client.HOCRText()
	default:
		text, err = e.client.Text()
	}
	if err != nil {
		return ocrmodel.Result{}, err
	}

	if err := ctx.Err(); err != nil {
		return ocrmodel.Result{}, err
	}

	return ocrmodel.Result{
		Text:   text,
		Format: request.Format,
	}, nil
}

func (e *Engine) Close() error {
	return e.client.Close()
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
