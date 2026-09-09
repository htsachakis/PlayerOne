// Package branding is the single source of truth for the application's identity.
//
// Renaming the application means editing the constants here, the "name" and
// "outputfilename" fields in wails.json, and the --wid host window class in
// internal/winvideo. Nothing else in the codebase hardcodes the name.
package branding

const (
	// Name is used for the window title, the config directory under %APPDATA%,
	// and the logger prefix. Keep it filesystem-safe: it becomes a directory name.
	Name = "PlayerOne"

	// Tagline appears in the empty state of the UI.
	Tagline = "Watch. Learn. Explore."

	// IPCPipePrefix namespaces the mpv control pipe. The process ID is appended so
	// multiple instances never collide.
	IPCPipePrefix = `\\.\pipe\playerone-`

	// VideoWindowClass is the Win32 class registered for the mpv host window.
	VideoWindowClass = "PlayerOneVideoHost"
)
