package ocr

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"testing"

	"github.com/KristinaKurian/ocrserver/internal/ocrmodel"
	"github.com/KristinaKurian/ocrserver/internal/workerpool"
)

type echoEngine struct{}

func (echoEngine) Recognize(_ context.Context, req ocrmodel.Request) (ocrmodel.Result, error) {
	return ocrmodel.Result{Text: "  text  \n", Format: req.Format}, nil
}
func (echoEngine) Close() error { return nil }

func TestServiceDefaultsToEnglishAndTrimsText(t *testing.T) {
	pool, err := workerpool.New(1, 1, func() (ocrmodel.Engine, error) { return echoEngine{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	service := NewService(pool, Limits{MaxWidth: 100, MaxHeight: 100, MaxPixels: 10_000})
	result, err := service.Recognize(context.Background(), ocrmodel.Request{
		Image:  testPNG(t, 10, 10),
		Format: ocrmodel.FormatText,
		Trim:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "text" {
		t.Fatalf("unexpected text: %q", result.Text)
	}
}

func TestServiceRejectsLargeDimensions(t *testing.T) {
	pool, err := workerpool.New(1, 1, func() (ocrmodel.Engine, error) { return echoEngine{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	service := NewService(pool, Limits{MaxWidth: 5, MaxHeight: 5, MaxPixels: 25})
	_, err = service.Recognize(context.Background(), ocrmodel.Request{
		Image: testPNG(t, 10, 10),
	})
	if !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("expected ErrImageTooLarge, got %v", err)
	}
}

func TestServiceRejectsInvalidLanguage(t *testing.T) {
	pool, err := workerpool.New(1, 1, func() (ocrmodel.Engine, error) { return echoEngine{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	service := NewService(pool, Limits{MaxWidth: 100, MaxHeight: 100, MaxPixels: 10_000})
	_, err = service.Recognize(context.Background(), ocrmodel.Request{
		Image:     testPNG(t, 10, 10),
		Languages: []string{"../../rus"},
	})
	if !errors.Is(err, ErrInvalidLanguage) {
		t.Fatalf("expected ErrInvalidLanguage, got %v", err)
	}
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
