package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"regexp"
	"strings"

	"github.com/KristinaKurian/ocrserver/internal/ocrmodel"
	"github.com/KristinaKurian/ocrserver/internal/workerpool"
)

var (
	ErrEmptyImage        = errors.New("image is required")
	ErrImageTooLarge     = errors.New("image dimensions are too large")
	ErrUnsupportedImage  = errors.New("unsupported image format")
	ErrInvalidImage      = errors.New("invalid image")
	ErrInvalidFormat     = errors.New("invalid OCR output format")
	ErrInvalidLanguage   = errors.New("invalid OCR language")
	ErrWhitelistTooLarge = errors.New("whitelist is too large")
)

var languageCodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type Limits struct {
	MaxWidth  int
	MaxHeight int
	MaxPixels int64
}

type Service struct {
	pool   *workerpool.Pool
	limits Limits
}

func NewService(pool *workerpool.Pool, limits Limits) *Service {
	return &Service{pool: pool, limits: limits}
}

func (s *Service) Recognize(ctx context.Context, request ocrmodel.Request) (ocrmodel.Result, error) {
	if len(request.Image) == 0 {
		return ocrmodel.Result{}, ErrEmptyImage
	}

	if request.Format == "" {
		request.Format = ocrmodel.FormatText
	}
	if request.Format != ocrmodel.FormatText && request.Format != ocrmodel.FormatHOCR {
		return ocrmodel.Result{}, ErrInvalidFormat
	}

	languages, err := normalizeLanguages(request.Languages)
	if err != nil {
		return ocrmodel.Result{}, err
	}
	request.Languages = languages

	if len(request.Whitelist) > 256 {
		return ocrmodel.Result{}, ErrWhitelistTooLarge
	}

	if err := s.validateImage(request.Image); err != nil {
		return ocrmodel.Result{}, err
	}

	result, err := s.pool.Process(ctx, request)
	if err != nil {
		return ocrmodel.Result{}, err
	}

	if request.Trim && result.Format == ocrmodel.FormatText {
		result.Text = strings.TrimSpace(result.Text)
	}

	return result, nil
}

func (s *Service) Ready() bool {
	return s != nil && s.pool != nil && s.pool.Ready()
}

func (s *Service) Stats() workerpool.Stats {
	if s == nil || s.pool == nil {
		return workerpool.Stats{}
	}
	return s.pool.Stats()
}

func (s *Service) validateImage(data []byte) error {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}

	if format != "png" && format != "jpeg" {
		return fmt.Errorf("%w: %s", ErrUnsupportedImage, format)
	}

	pixels := int64(cfg.Width) * int64(cfg.Height)
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return ErrInvalidImage
	}
	if s.limits.MaxWidth > 0 && cfg.Width > s.limits.MaxWidth {
		return ErrImageTooLarge
	}
	if s.limits.MaxHeight > 0 && cfg.Height > s.limits.MaxHeight {
		return ErrImageTooLarge
	}
	if s.limits.MaxPixels > 0 && pixels > s.limits.MaxPixels {
		return ErrImageTooLarge
	}
	return nil
}

func normalizeLanguages(languages []string) ([]string, error) {
	if len(languages) == 0 {
		return []string{"eng"}, nil
	}

	result := make([]string, 0, len(languages))
	seen := make(map[string]struct{}, len(languages))
	for _, language := range languages {
		language = strings.TrimSpace(language)
		if language == "" {
			continue
		}
		if !languageCodePattern.MatchString(language) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidLanguage, language)
		}
		if _, ok := seen[language]; ok {
			continue
		}
		seen[language] = struct{}{}
		result = append(result, language)
	}

	if len(result) == 0 {
		return []string{"eng"}, nil
	}
	return result, nil
}
