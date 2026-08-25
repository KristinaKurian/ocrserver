package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port              string
	Workers           int
	QueueSize         int
	MaxImageBytes     int64
	MaxImageWidth     int
	MaxImageHeight    int
	MaxImagePixels    int64
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

func Load() Config {
	maxImageMB := envInt("OCR_MAX_IMAGE_MB", 20)

	return Config{
		Port:              envString("PORT", "8080"),
		Workers:           envInt("OCR_WORKERS", 4),
		QueueSize:         envInt("OCR_QUEUE_SIZE", 8),
		MaxImageBytes:     int64(maxImageMB) << 20,
		MaxImageWidth:     envInt("OCR_MAX_IMAGE_WIDTH", 10000),
		MaxImageHeight:    envInt("OCR_MAX_IMAGE_HEIGHT", 10000),
		MaxImagePixels:    int64(envInt("OCR_MAX_IMAGE_PIXELS", 40_000_000)),
		ReadHeaderTimeout: envDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		ReadTimeout:       envDuration("HTTP_READ_TIMEOUT", 30*time.Second),
		WriteTimeout:      envDuration("HTTP_WRITE_TIMEOUT", 120*time.Second),
		IdleTimeout:       envDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout:   envDuration("HTTP_SHUTDOWN_TIMEOUT", 30*time.Second),
	}
}

func envString(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}
