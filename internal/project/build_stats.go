package project

import (
	"fmt"
	"sync/atomic"
	"time"
)

// BuildStats holds build metrics and errors
type BuildStats struct {
	Scheduled int
	Completed int64
	CacheHits int64
	Written   int64
	Skipped   int64
	Duration  time.Duration
	Errors    []error
}

func (b *BuildStats) String() string {
	return fmt.Sprintf(
		"Build: %d/%d entries rebuilt (%d from cache, %d errors, %d skipped) in %s",
		b.Completed, b.Scheduled, b.CacheHits, len(b.Errors), b.Skipped, b.Duration.String(),
	)
}

// buildMetrics tracks build metrics while building. At the end, a BuildResult can be used
type buildMetrics struct {
	built   atomic.Int64
	written atomic.Int64
	skipped atomic.Int64
	cached  atomic.Int64
	start   time.Time
}

func (m *buildMetrics) Snapshot() BuildStats {
	return BuildStats{
		Completed: m.built.Load(),
		CacheHits: m.cached.Load(),
		Written:   m.written.Load(),
		Skipped:   m.skipped.Load(),
		Duration:  time.Since(m.start),
	}
}
