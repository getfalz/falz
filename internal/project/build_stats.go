package project

import (
	"fmt"
	"sync/atomic"
)

// BuildStats holds build metrics and errors
type BuildStats struct {
	Total int
	Built  int64
	Cached int64
	Errors []error
}

func (b *BuildStats) String() string {
	return fmt.Sprintf(
		"Build: %d/%d entries rebuilt (%d from cache, %d errors)",
		b.Built, b.Total, b.Cached, len(b.Errors),
	)
}

// buildMetrics tracks build metrics while building. At the end, a BuildResult can be used
type buildMetrics struct {
	built     atomic.Int64
	written   atomic.Int64
	cached atomic.Int64
}

func (m *buildMetrics) Snapshot() BuildStats {
	return BuildStats{
		Built:  m.built.Load(),
		Cached: m.cached.Load(),
	}
}
