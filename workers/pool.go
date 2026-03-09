package workers

import (
	"context"
	"sync"
)

type Job[T, R any] struct {
	ID      string
	Input   T
	Process func(ctx context.Context, input T) (R, error)
}

type Result[R any] struct {
	JobID  string
	Output R
	Err    error
}

type WorkerPool[T, R any] struct {
	workers  int
	jobCh    chan Job[T, R]
	resultCh chan Result[R]
	wg       sync.WaitGroup

	startOnce sync.Once
	closeOnce sync.Once
}

func NewWorkerPool[T, R any](workers int) *WorkerPool[T, R] {
	if workers <= 0 {
		workers = 1
	}

	return &WorkerPool[T, R]{
		workers:  workers,
		jobCh:    make(chan Job[T, R]),
		resultCh: make(chan Result[R]),
	}
}

func (p *WorkerPool[T, R]) Start(ctx context.Context) {
	p.startOnce.Do(func() {
		for i := 0; i < p.workers; i++ {
			p.wg.Add(1)
			go p.worker(ctx)
		}

		go func() {
			p.wg.Wait()
			close(p.resultCh)
		}()
	})
}

func (p *WorkerPool[T, R]) Submit(job Job[T, R]) {
	p.jobCh <- job
}

func (p *WorkerPool[T, R]) Results() <-chan Result[R] {
	return p.resultCh
}

func (p *WorkerPool[T, R]) Wait() {
	p.wg.Wait()
}

func (p *WorkerPool[T, R]) Close() {
	p.closeOnce.Do(func() {
		close(p.jobCh)
	})
}

func (p *WorkerPool[T, R]) worker(ctx context.Context) {
	defer p.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-p.jobCh:
			if !ok {
				return
			}

			var out R
			var err error
			if job.Process != nil {
				out, err = job.Process(ctx, job.Input)
			}

			select {
			case <-ctx.Done():
				return
			case p.resultCh <- Result[R]{
				JobID:  job.ID,
				Output: out,
				Err:    err,
			}:
			}
		}
	}
}
