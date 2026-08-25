FROM golang:1.27-bookworm AS builder

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        libtesseract-dev \
        libleptonica-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/ocrserver ./cmd/ocrserver

FROM debian:bookworm-slim

LABEL org.opencontainers.image.title="OCR Server"
LABEL org.opencontainers.image.source="https://github.com/KristinaKurian/ocrserver"

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        tesseract-ocr \
        tesseract-ocr-eng \
        tesseract-ocr-rus \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system app \
    && useradd --system --gid app --no-create-home app

COPY --from=builder /out/ocrserver /usr/local/bin/ocrserver

USER app

ENV PORT=8080

EXPOSE 8080

CMD ["ocrserver"]
