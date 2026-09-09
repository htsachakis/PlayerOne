//go:build !windows

package mpvipc

import (
	"context"
	"net"
)

// dialPipe opens mpv's IPC socket. On non-Windows platforms --input-ipc-server
// creates a Unix domain socket rather than a named pipe.
//
// PlayerOne targets Windows, but keeping the transport buildable elsewhere means
// the package's tests run on any platform and the code stays honest about which
// parts are genuinely Windows-specific (only internal/winvideo is).
func dialPipe(ctx context.Context, name string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", name)
}
