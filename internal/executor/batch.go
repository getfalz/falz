package executor

import (
	"context"
	"errors"
	"runtime"
	"sync"

	"golang.org/x/sync/errgroup"
)

type Task interface {
	Key() string
	Run(ctx context.Context) error
}

type BatchExecutor struct {
	Concurrency int
	BestEffort  bool
}

func (e BatchExecutor) Run(ctx context.Context, tasks []Task) error {
	if e.Concurrency <= 0 {
		e.Concurrency = runtime.NumCPU()
	}
	var g errgroup.Group
	g.SetLimit(e.Concurrency)

	if !e.BestEffort {
		for i := range tasks {
			t := tasks[i]
			g.Go(func() error { return t.Run(ctx) })
		}
		return g.Wait()
	}

	var mu sync.Mutex
	var errs []error
	for i := range tasks {
		t := tasks[i]
		g.Go(func() error {
			if err := t.Run(ctx); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()
	return errors.Join(errs...)
}
