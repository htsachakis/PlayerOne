//go:build !windows

package updater

import "errors"

// ErrUnsupported is returned where installing an update is not implemented.
var ErrUnsupported = errors.New("updater: installing updates is only implemented on Windows")

// IsInstalled reports whether this copy came from an installer.
func IsInstalled() bool { return false }

// Launch is not available on this platform.
func Launch(string) error { return ErrUnsupported }
