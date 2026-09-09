//go:build !windows

package media

import "os/exec"

// configureProcAttr has nothing to do outside Windows.
func configureProcAttr(*exec.Cmd) {}
