package utils

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"craeft/internal/log"
)

func NormPath(p string) string {
	// Your matches from fs.Glob are slash-separated already, but normalize anyway.
	p = strings.ReplaceAll(p, "\\", "/") // just in case
	p = path.Clean(p)
	if p == "." {
		return "."
	}
	return strings.TrimPrefix(p, "./")
}

// TODO deprecate
func StopComponent(ctx context.Context, name string, stop func(context.Context) error) error {
	var lg = log.Logger().With().Str("phase", "shutdown").Str("component", name).Logger()

	begin := time.Now()

	lg.Debug().Msg("stopping")
	err := stop(ctx)
	dur := time.Since(begin)

	if err != nil {
		lg.Warn().Dur("dur", dur).Err(err).Msg("failed")
		return fmt.Errorf("stop %s: %w", name, err)
	}
	lg.Debug().Dur("dur", dur).Msg("stopped")
	return nil
}
