//go:build windows

package media

import (
	"os/exec"
	"syscall"
)

// configureProcAttr keeps ffmpeg and ffprobe from flashing a console window.
// Both are console applications, so without this every probe and every subtitle
// extraction would blink a black window over the player.
func configureProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
