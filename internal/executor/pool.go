package executor

import (
	"context"
	"runtime"
	"sync"
	"time"
)

type Debouncer interface {
	Submit(key string, fn func())
}

type TimerDebouncer struct {
	dur    time.Duration
	mu     sync.Mutex
	timers map[string]*time.Timer
}

func NewTimerDebouncer(d time.Duration) *TimerDebouncer {
	return &TimerDebouncer{dur: d, timers: make(map[string]*time.Timer)}
}
func (d *TimerDebouncer) Submit(key string, fn func()) {
	d.mu.Lock()
	if tm, ok := d.timers[key]; ok {
		tm.Reset(d.dur)
		d.mu.Unlock()
		return
	}
	tm := time.AfterFunc(d.dur, func() {
		fn()
		d.mu.Lock()
		delete(d.timers, key)
		d.mu.Unlock()
	})
	d.timers[key] = tm
	d.mu.Unlock()
}

type PoolExecutor struct {
	Concurrency int
	QueueSize   int
	Debounce    Debouncer

	ch      chan Task
	wg      sync.WaitGroup
	started bool
	mu      sync.Mutex
}

func NewPoolExecutor(concurrency, queue int, d Debouncer) *PoolExecutor {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}
	if queue <= 0 {
		queue = 1024
	}
	return &PoolExecutor{
		Concurrency: concurrency,
		QueueSize:   queue,
		Debounce:    d,
		ch:          make(chan Task, queue),
	}
}

func (p *PoolExecutor) Start(ctx context.Context) {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	p.mu.Unlock()

	for i := 0; i < p.Concurrency; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case t := <-p.ch:
					_ = t.Run(ctx)
				}
			}
		}()
	}
}

func (p *PoolExecutor) Stop() {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return
	}
	close(p.ch)
	p.started = false
	p.mu.Unlock()
	p.wg.Wait()
}

func (p *PoolExecutor) Submit(t Task) {
	if p.Debounce == nil {
		select {
		case p.ch <- t:
		default:
		}
		return
	}
	p.Debounce.Submit(t.Key(), func() {
		select {
		case p.ch <- t:
		default:
		}
	})
}
