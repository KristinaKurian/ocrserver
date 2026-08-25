package workerpool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/KristinaKurian/ocrserver/internal/ocrmodel"
)

var (
	ErrQueueFull      = errors.New("OCR queue is full")
	ErrClosed         = errors.New("OCR worker pool is closed")
	ErrMissingFactory = errors.New("OCR engine factory is nil")
)

type Stats struct {
	Workers       int
	QueueLength   int
	QueueCapacity int
	Rejected      uint64
}

type job struct {
	ctx      context.Context
	request  ocrmodel.Request
	response chan result
}

type result struct {
	value ocrmodel.Result
	err   error
}

type worker struct {
	engine ocrmodel.Engine
}

type Pool struct {
	jobs     chan job
	stop     chan struct{}
	once     sync.Once
	wg       sync.WaitGroup
	workers  []worker
	rejected atomic.Uint64
	closed   atomic.Bool
}

func New(workerCount, queueSize int, factory ocrmodel.EngineFactory) (*Pool, error) {
	if factory == nil {
		return nil, ErrMissingFactory
	}
	if workerCount <= 0 {
		workerCount = 1
	}
	if queueSize <= 0 {
		queueSize = workerCount
	}

	p := &Pool{
		jobs:    make(chan job, queueSize),
		stop:    make(chan struct{}),
		workers: make([]worker, 0, workerCount),
	}

	for i := 0; i < workerCount; i++ {
		engine, err := factory()
		if err != nil {
			for _, w := range p.workers {
				_ = w.engine.Close()
			}
			return nil, err
		}
		p.workers = append(p.workers, worker{engine: engine})
	}

	p.wg.Add(len(p.workers))
	for i := range p.workers {
		go p.runWorker(&p.workers[i])
	}

	return p, nil
}

// Process uses non-blocking admission. When all workers are busy and the
// bounded queue is full, ErrQueueFull is returned immediately instead of
// accumulating an unbounded number of waiting HTTP goroutines.
func (p *Pool) Process(ctx context.Context, request ocrmodel.Request) (ocrmodel.Result, error) {
	if p == nil || p.closed.Load() {
		return ocrmodel.Result{}, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return ocrmodel.Result{}, err
	}

	response := make(chan result, 1)
	j := job{ctx: ctx, request: request, response: response}

	select {
	case <-p.stop:
		return ocrmodel.Result{}, ErrClosed
	case p.jobs <- j:
	default:
		p.rejected.Add(1)
		return ocrmodel.Result{}, ErrQueueFull
	}

	select {
	case <-ctx.Done():
		return ocrmodel.Result{}, ctx.Err()
	case <-p.stop:
		return ocrmodel.Result{}, ErrClosed
	case res := <-response:
		return res.value, res.err
	}
}

func (p *Pool) Stats() Stats {
	if p == nil {
		return Stats{}
	}
	return Stats{
		Workers:       len(p.workers),
		QueueLength:   len(p.jobs),
		QueueCapacity: cap(p.jobs),
		Rejected:      p.rejected.Load(),
	}
}

func (p *Pool) Ready() bool {
	return p != nil && !p.closed.Load()
}

func (p *Pool) Close() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		p.closed.Store(true)
		close(p.stop)
		p.wg.Wait()
	})
}

func (p *Pool) runWorker(w *worker) {
	defer p.wg.Done()
	defer w.engine.Close()

	for {
		select {
		case <-p.stop:
			return
		case j := <-p.jobs:
			if err := j.ctx.Err(); err != nil {
				continue
			}

			value, err := w.engine.Recognize(j.ctx, j.request)
			res := result{value: value, err: err}

			select {
			case j.response <- res:
			case <-j.ctx.Done():
			case <-p.stop:
				return
			}
		}
	}
}
