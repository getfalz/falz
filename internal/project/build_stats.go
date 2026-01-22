package project

import (
	"fmt"
	"sync/atomic"
	"time"
)

type RunStats struct {
	PlanDuration  time.Duration
	BuildDuration time.Duration
	BuildStats
}

// BuildStats tracks build statistics
type BuildStats struct {
	TotalEntries int
	DirtyEntries int

	BuildSummary
}

// BuildSummary holds build metrics and errors
type BuildSummary struct {
	Built        int64
	CacheHits    int64
	CacheMisses  int64
	BytesWritten int64
	BytesCached  int64
	Errors       []error
}

func (b *RunStats) String() string {
	return fmt.Sprintf(
		"[Stats]: Plan: %d, Build: %d, \n%s",
		b.PlanDuration, b.BuildDuration, &b.BuildStats,
	)
}
func (b *BuildStats) String() string {
	return fmt.Sprintf(
		"[Build Entries]: Total: %d, Dirty: %d,\n%s",
		b.TotalEntries, b.DirtyEntries, &b.BuildSummary,
	)
}

func (b *BuildSummary) String() string {
	return fmt.Sprintf(
		"Total Built: %d, Cache Hits: %d, Cache Misses: %d, Bytes Written: %d, Bytes Cached: %d",
		b.Built, b.CacheHits, b.CacheMisses, b.BytesWritten, b.BytesCached,
	)
}

// buildMetrics tracks build metrics while building. At the end, a BuildResult can be used
type buildMetrics struct {
	processed    atomic.Int64
	written      atomic.Int64
	cacheHits    atomic.Int64
	cacheMisses  atomic.Int64
	bytesWritten atomic.Int64
	bytesCached  atomic.Int64
}

func (m *buildMetrics) Snapshot() BuildSummary {
	return BuildSummary{
		Built:        m.processed.Load(),
		CacheHits:    m.cacheHits.Load(),
		CacheMisses:  m.cacheMisses.Load(),
		BytesWritten: m.bytesWritten.Load(),
		BytesCached:  m.bytesCached.Load(),
	}
}
