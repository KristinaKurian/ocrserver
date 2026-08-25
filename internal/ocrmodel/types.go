package ocrmodel

import "context"

type Format string

const (
	FormatText Format = "text"
	FormatHOCR Format = "hocr"
)

type Request struct {
	Image     []byte
	Languages []string
	Whitelist string
	Format    Format
	Trim      bool
}

type Result struct {
	Text   string `json:"text"`
	Format Format `json:"format"`
}

type Engine interface {
	Recognize(ctx context.Context, request Request) (Result, error)
	Close() error
}

type EngineFactory func() (Engine, error)
