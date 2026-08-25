package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KristinaKurian/ocrserver/internal/config"
	"github.com/KristinaKurian/ocrserver/internal/httpapi"
	"github.com/KristinaKurian/ocrserver/internal/ocr"
	"github.com/KristinaKurian/ocrserver/internal/tesseract"
	"github.com/KristinaKurian/ocrserver/internal/workerpool"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	pool, err := workerpool.New(cfg.Workers, cfg.QueueSize, tesseract.New)
	if err != nil {
		logger.Error("failed to start OCR worker pool", "error", err)
		os.Exit(1)
	}

	service := ocr.NewService(pool, ocr.Limits{
		MaxWidth:  cfg.MaxImageWidth,
		MaxHeight: cfg.MaxImageHeight,
		MaxPixels: cfg.MaxImagePixels,
	})

	handler := httpapi.New(service, httpapi.Config{
		MaxImageBytes: cfg.MaxImageBytes,
	}, logger)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("OCR server started",
			"address", server.Addr,
			"workers", cfg.Workers,
			"queue_size", cfg.QueueSize,
			"max_image_mb", cfg.MaxImageBytes>>20,
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		if err != nil {
			logger.Error("HTTP server failed", "error", err)
			pool.Close()
			os.Exit(1)
		}
	case <-signalCtx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP graceful shutdown failed", "error", err)
	}

	pool.Close()
	logger.Info("OCR server stopped", "time", time.Now().UTC())
}
