//go:build !windows

package player

import "os/exec"

// configureProcAttr has nothing to do outside Windows; console windows are not
// a concern here.
func configureProcAttr(*exec.Cmd) {}
