package workerpool

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/KristinaKurian/ocrserver/internal/ocrmodel"
)

type blockingEngine struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (e *blockingEngine) Recognize(ctx context.Context, request ocrmodel.Request) (ocrmodel.Result, error) {
	e.once.Do(func() { close(e.started) })
	select {
	case <-e.release:
		return ocrmodel.Result{Text: "ok", Format: request.Format}, nil
	case <-ctx.Done():
		return ocrmodel.Result{}, ctx.Err()
	}
}

func (e *blockingEngine) Close() error { return nil }

func TestPoolRejectsWhenQueueIsFull(t *testing.T) {
	engine := &blockingEngine{started: make(chan struct{}), release: make(chan struct{})}
	pool, err := New(1, 1, func() (ocrmodel.Engine, error) { return engine, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	firstDone := make(chan error, 1)
	go func() {
		_, err := pool.Process(context.Background(), ocrmodel.Request{Format: ocrmodel.FormatText})
		firstDone <- err
	}()

	select {
	case <-engine.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := pool.Process(context.Background(), ocrmodel.Request{Format: ocrmodel.FormatText})
		secondDone <- err
	}()

	deadline := time.Now().Add(time.Second)
	for pool.Stats().QueueLength != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	_, err = pool.Process(context.Background(), ocrmodel.Request{Format: ocrmodel.FormatText})
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}

	close(engine.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second request failed: %v", err)
	}
}

func TestPoolHonorsCancelledContext(t *testing.T) {
	engine := &blockingEngine{started: make(chan struct{}), release: make(chan struct{})}
	pool, err := New(1, 1, func() (ocrmodel.Engine, error) { return engine, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = pool.Process(ctx, ocrmodel.Request{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	close(engine.release)
}
