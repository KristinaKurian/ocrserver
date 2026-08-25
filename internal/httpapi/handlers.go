package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/KristinaKurian/ocrserver/internal/ocr"
	"github.com/KristinaKurian/ocrserver/internal/ocrmodel"
	"github.com/KristinaKurian/ocrserver/internal/workerpool"
)

const formFieldLimit = 4096

type jsonOCRRequest struct {
	ImageBase64 string          `json:"image_base64"`
	Languages   []string        `json:"languages"`
	Whitelist   string          `json:"whitelist"`
	Format      ocrmodel.Format `json:"format"`
	Trim        *bool           `json:"trim,omitempty"`
}

type errorBody struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (a *API) handleOCR(w http.ResponseWriter, r *http.Request) {
	request, err := a.readOCRRequest(w, r)
	if err != nil {
		a.writeRequestError(w, err)
		return
	}

	result, err := a.service.Recognize(r.Context(), request)
	if err != nil {
		a.writeOCRError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result, result.Format == ocrmodel.FormatHOCR)
}

func (a *API) readOCRRequest(w http.ResponseWriter, r *http.Request) (ocrmodel.Request, error) {
	contentType := r.Header.Get("Content-Type")

	switch {
	case strings.HasPrefix(contentType, "multipart/form-data"):
		return a.readMultipartRequest(w, r)
	case strings.HasPrefix(contentType, "application/json"):
		return a.readJSONRequest(w, r)
	default:
		return ocrmodel.Request{}, errUnsupportedContentType
	}
}

func (a *API) readMultipartRequest(w http.ResponseWriter, r *http.Request) (ocrmodel.Request, error) {
	r.Body = http.MaxBytesReader(w, r.Body, a.config.MaxImageBytes+(1<<20))

	reader, err := r.MultipartReader()
	if err != nil {
		return ocrmodel.Request{}, fmt.Errorf("invalid multipart request: %w", err)
	}

	request := ocrmodel.Request{Format: ocrmodel.FormatText, Trim: true}
	var fileFound bool

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ocrmodel.Request{}, err
		}

		name := part.FormName()
		if name == "file" {
			if fileFound {
				part.Close()
				continue
			}
			data, err := readLimited(part, a.config.MaxImageBytes)
			part.Close()
			if err != nil {
				return ocrmodel.Request{}, err
			}
			request.Image = data
			fileFound = true
			continue
		}

		value, err := readSmallField(part)
		part.Close()
		if err != nil {
			return ocrmodel.Request{}, err
		}
		switch name {
		case "languages":
			request.Languages = splitLanguages(value)
		case "whitelist":
			request.Whitelist = value
		case "format":
			request.Format = ocrmodel.Format(strings.ToLower(strings.TrimSpace(value)))
		case "trim":
			if value != "" {
				trim, err := strconv.ParseBool(value)
				if err != nil {
					return ocrmodel.Request{}, fmt.Errorf("invalid trim value: %w", err)
				}
				request.Trim = trim
			}
		}
	}

	if !fileFound {
		return ocrmodel.Request{}, ocr.ErrEmptyImage
	}
	return request, nil
}

func (a *API) readJSONRequest(w http.ResponseWriter, r *http.Request) (ocrmodel.Request, error) {
	// Base64 expands binary data by ~4/3. Add 2 MiB for JSON overhead.
	maxJSONBytes := a.config.MaxImageBytes*4/3 + (2 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBytes)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var body jsonOCRRequest
	if err := decoder.Decode(&body); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return ocrmodel.Request{}, errRequestTooLarge
		}
		return ocrmodel.Request{}, fmt.Errorf("invalid JSON: %w", err)
	}

	encoded := strings.TrimSpace(body.ImageBase64)
	if strings.HasPrefix(encoded, "data:") {
		comma := strings.IndexByte(encoded, ',')
		if comma < 0 {
			return ocrmodel.Request{}, errors.New("invalid data URI")
		}
		encoded = encoded[comma+1:]
	}

	imageBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ocrmodel.Request{}, fmt.Errorf("invalid base64 image: %w", err)
	}
	if int64(len(imageBytes)) > a.config.MaxImageBytes {
		return ocrmodel.Request{}, errImageBytesTooLarge
	}

	format := body.Format
	if format == "" {
		format = ocrmodel.FormatText
	}
	trim := true
	if body.Trim != nil {
		trim = *body.Trim
	}

	return ocrmodel.Request{
		Image:     imageBytes,
		Languages: body.Languages,
		Whitelist: body.Whitelist,
		Format:    format,
		Trim:      trim,
	}, nil
}

func readLimited(reader io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errImageBytesTooLarge
	}
	return data, nil
}

func readSmallField(part *multipart.Part) (string, error) {
	data, err := io.ReadAll(io.LimitReader(part, formFieldLimit+1))
	if err != nil {
		return "", err
	}
	if len(data) > formFieldLimit {
		return "", errors.New("form field is too large")
	}
	return string(data), nil
}

func splitLanguages(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Split(value, ",")
}

var (
	errUnsupportedContentType = errors.New("content type must be multipart/form-data or application/json")
	errRequestTooLarge        = errors.New("request body is too large")
	errImageBytesTooLarge     = errors.New("image file is too large")
)

func (a *API) writeRequestError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		writeAPIError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body is too large")
		return
	}

	switch {
	case errors.Is(err, errUnsupportedContentType):
		writeAPIError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_CONTENT_TYPE", err.Error())
	case errors.Is(err, errRequestTooLarge), errors.Is(err, errImageBytesTooLarge):
		writeAPIError(w, http.StatusRequestEntityTooLarge, "IMAGE_TOO_LARGE", err.Error())
	case errors.Is(err, ocr.ErrEmptyImage):
		writeAPIError(w, http.StatusBadRequest, "IMAGE_REQUIRED", err.Error())
	default:
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
	}
}

func (a *API) writeOCRError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, workerpool.ErrQueueFull):
		w.Header().Set("Retry-After", "1")
		writeAPIError(w, http.StatusServiceUnavailable, "OCR_QUEUE_FULL", "OCR service is busy; retry later")
	case errors.Is(err, workerpool.ErrClosed):
		writeAPIError(w, http.StatusServiceUnavailable, "OCR_NOT_READY", "OCR service is shutting down")
	case errors.Is(err, ocr.ErrEmptyImage):
		writeAPIError(w, http.StatusBadRequest, "IMAGE_REQUIRED", err.Error())
	case errors.Is(err, ocr.ErrImageTooLarge):
		writeAPIError(w, http.StatusRequestEntityTooLarge, "IMAGE_DIMENSIONS_TOO_LARGE", err.Error())
	case errors.Is(err, ocr.ErrUnsupportedImage):
		writeAPIError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_IMAGE", err.Error())
	case errors.Is(err, ocr.ErrInvalidImage), errors.Is(err, ocr.ErrInvalidFormat), errors.Is(err, ocr.ErrInvalidLanguage), errors.Is(err, ocr.ErrWhitelistTooLarge):
		writeAPIError(w, http.StatusBadRequest, "INVALID_OCR_REQUEST", err.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeAPIError(w, http.StatusRequestTimeout, "REQUEST_CANCELLED", "request cancelled")
	default:
		a.logger.Error("OCR failed", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "OCR_FAILED", "OCR processing failed")
	}
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: apiError{Code: code, Message: message}}, false)
}

func writeJSON(w http.ResponseWriter, status int, value any, disableHTMLEscape bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	if disableHTMLEscape {
		encoder.SetEscapeHTML(false)
	}
	_ = encoder.Encode(value)
}
