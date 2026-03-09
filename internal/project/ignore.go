package project

import (
	"craeft/internal/log"
	"path"
	"path/filepath"
	"strings"
)

type IgnoreList []string

// TODO rename to Match
func (il IgnoreList) Match(name string) bool {
	name = filepath.ToSlash(filepath.Clean(name))

	for _, raw := range il {
		pat := filepath.ToSlash(strings.TrimSpace(raw))
		if pat == "" {
			continue
		}

		if strings.HasSuffix(pat, "/**") {
			prefix := strings.TrimSuffix(pat, "/**")
			if name == prefix || strings.HasPrefix(name, prefix+"/") {
				log.TTrace(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by prefix/**")
				return true
			}
			continue
		}
		if !strings.ContainsAny(pat, "*?[]") {
			if name == pat || strings.HasPrefix(name, pat+"/") {
				log.TTrace(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by exact/prefix")
				return true
			}
			continue
		}

		ok, err := path.Match(pat, name)
		if err != nil {
			continue
		}
		if ok {
			log.TTrace(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by glob")
			return true
		}
	}
	return false
}
