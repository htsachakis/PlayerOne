//go:build windows

package mpvipc

import (
	"context"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// dialPipe opens mpv's --input-ipc-server named pipe.
//
// go-winio is used rather than opening the pipe by hand because a named pipe
// that exists but has no server listening yet returns ERROR_PIPE_BUSY, which
// needs the WaitNamedPipe dance to handle correctly.
func dialPipe(ctx context.Context, name string) (net.Conn, error) {
	// A short per-attempt timeout keeps Dial's retry loop responsive; the
	// caller's context governs how long we keep retrying overall.
	timeout := 500 * time.Millisecond
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		timeout = time.Millisecond
	}

	return winio.DialPipe(name, &timeout)
}
