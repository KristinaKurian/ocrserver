# OCR Server

A Go OCR API built around Tesseract with bounded concurrency, backpressure,
input validation, health checks, metrics, and graceful shutdown.

The service is intentionally backend-only. It uses the Go standard HTTP server
instead of a web framework and isolates Tesseract behind an `Engine` interface.

## Architecture

```text
HTTP API
   |
   v
OCR Service
   |
   v
Bounded Worker Pool
   |
   v
OCR Engine interface
   |
   v
Tesseract adapter (gosseract)
```

Each worker owns exactly one Tesseract client and processes jobs sequentially.
The HTTP layer never uses `gosseract` directly.

## Run

```sh
docker compose up --build
```

The API listens on `http://localhost:8080`.

The Docker image contains both English and Russian Tesseract language data.

Check installed languages:

```sh
docker compose exec web tesseract --list-langs
```

You should see at least:

```text
eng
rus
```

## API

### POST `/api/v1/ocr`

The endpoint accepts either `multipart/form-data` or JSON with a base64 image.

Multipart example:

```sh
curl -X POST http://localhost:8080/api/v1/ocr \
  -F "file=@example.png" \
  -F "languages=rus,eng" \
  -F "format=text"
```

hOCR:

```sh
curl -X POST http://localhost:8080/api/v1/ocr \
  -F "file=@example.png" \
  -F "languages=rus" \
  -F "format=hocr"
```

JSON/base64 example:

```json
{
  "image_base64": "iVBORw0KGgoAAA...",
  "languages": ["rus", "eng"],
  "format": "text",
  "trim": true
}
```

Response:

```json
{
  "text": "Распознанный текст",
  "format": "text"
}
```

### GET `/health/live`

Process liveness probe.

### GET `/health/ready`

Reports whether the OCR worker pool is ready and includes queue information.

### GET `/metrics`

Exposes Prometheus text-format metrics without an additional metrics library.

## Backpressure

The number of concurrent OCR calls and queued jobs is bounded:

```text
OCR_WORKERS=4
OCR_QUEUE_SIZE=8
```

When the queue is full, new OCR requests are rejected immediately with:

```text
503 Service Unavailable
OCR_QUEUE_FULL
```

This prevents an unbounded number of HTTP goroutines from waiting for OCR.

## Input limits

Defaults:

```text
OCR_MAX_IMAGE_MB=20
OCR_MAX_IMAGE_WIDTH=10000
OCR_MAX_IMAGE_HEIGHT=10000
OCR_MAX_IMAGE_PIXELS=40000000
```

Only PNG and JPEG images are accepted. The service checks both encoded byte
size and decoded image dimensions before submitting work to Tesseract.

## Configuration

| Variable | Default | Description |
| --- | ---: | --- |
| `PORT` | `8080` | HTTP port |
| `OCR_WORKERS` | `4` | Concurrent Tesseract engines |
| `OCR_QUEUE_SIZE` | `8` | Maximum queued OCR jobs |
| `OCR_MAX_IMAGE_MB` | `20` | Maximum decoded input bytes |
| `OCR_MAX_IMAGE_WIDTH` | `10000` | Maximum image width |
| `OCR_MAX_IMAGE_HEIGHT` | `10000` | Maximum image height |
| `OCR_MAX_IMAGE_PIXELS` | `40000000` | Maximum width × height |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | HTTP header timeout |
| `HTTP_READ_TIMEOUT` | `30s` | HTTP read timeout |
| `HTTP_WRITE_TIMEOUT` | `120s` | HTTP write timeout |
| `HTTP_IDLE_TIMEOUT` | `60s` | Keep-alive idle timeout |
| `HTTP_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown timeout |

## Project layout

```text
cmd/ocrserver/          application entry point
internal/config/        environment configuration
internal/httpapi/       HTTP handlers, middleware, health and metrics
internal/ocr/           OCR use-case/service and input validation
internal/ocrmodel/      engine-neutral OCR types and interface
internal/tesseract/     gosseract/Tesseract adapter
internal/workerpool/    bounded worker pool and backpressure
```

## Development

Install Tesseract development libraries, then:

```sh
go test ./...
go run ./cmd/ocrserver
```
