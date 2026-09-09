//go:build windows

package player

import (
	"os/exec"
	"syscall"
)

// configureProcAttr stops mpv from flashing a console window.
//
// mpv.exe is a GUI subsystem binary, but it is launched here with no console of
// its own; CREATE_NO_WINDOW makes that explicit and also covers mpv.com, which
// is a console binary and would otherwise pop up a black window.
func configureProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
