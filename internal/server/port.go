package server

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"runtime"
	"time"

	"craeft/internal/log"
)

const portRetries = 49

func candidatePorts(base, retries int) []int {
	total := retries + 1 // original port + additional retries
	ports := make([]int, 0, total)
	for i := 0; i < total; i++ {
		var p int
		if i < 5 {
			p = base + i
		} else {
			delta := rand.Intn(4*total+1) - 2*total
			p = base + delta
			if p < 1 {
				p = 1
			}
			if p > 65535 {
				p = 65535
			}
		}
		ports = append(ports, p)
	}
	return ports
}

func listenSearchPort(host string, basePort, retries int) (net.Listener, int, error) {
	var errs []error
	for _, port := range candidatePorts(basePort, retries) {
		addr := fmt.Sprintf("%s:%d", host, port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			if port != basePort {
				log.WWarn(log.Server).
					Int("port", port).
					Msg("the port is already in use; using a different port instead")
			}
			return ln, port, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", addr, err))
	}
	return nil, 0, fmt.Errorf(
		"no available port found near %d after %d attempts: %w",
		basePort, retries+1, errors.Join(errs...),
	)
}

func openBrowser(url string) bool {
	var cmds [][]string

	if b := os.Getenv("BROWSER"); b != "" {
		cmds = append(cmds, []string{b})
	}

	switch runtime.GOOS {
	case "darwin":
		cmds = append(cmds, []string{"/usr/bin/open"})
	case "windows":
		cmds = append(cmds, []string{"cmd", "/c", "start"})
	default:
		if os.Getenv("DISPLAY") != "" {
			cmds = append(cmds, []string{"xdg-open"})
		}
	}

	cmds = append(cmds,
		[]string{"chrome"},
		[]string{"google-chrome"},
		[]string{"chromium"},
		[]string{"firefox"},
	)

	for _, args := range cmds {
		cmd := exec.Command(args[0], append(args[1:], url)...)
		if cmd.Start() == nil {
			return true
		}
	}

	return false
}

func tryOpenBrowser(ctx context.Context, url string) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(300 * time.Millisecond):
	}
	if openBrowser(url) {
		log.DDebug(log.Server).Str("url", url).Msg("could not open browser; ignoring")
		return
	}
	log.IInfo(log.Server).Str("url", url).Msg("To access the server, open this URL in your browser")
}
