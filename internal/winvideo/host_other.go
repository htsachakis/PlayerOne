//go:build !windows

// Package winvideo owns the native child window that mpv renders into.
//
// PlayerOne targets Windows. This stub exists so the rest of the codebase stays
// buildable and testable elsewhere, and so the Windows-only surface is confined
// to one small, obvious place rather than spreading through the application.
package winvideo

import (
	"errors"
	"time"
)

// ErrUnsupported is returned by every operation on platforms without an
// implementation.
var ErrUnsupported = errors.New("winvideo: embedding the video window is only implemented on Windows")

// Bounds is a rectangle in physical pixels, relative to the parent's client
// area.
type Bounds struct {
	X, Y, Width, Height int
}

// Host is the native child window mpv renders into.
type Host struct{}

// NewHost is not available on this platform.
func NewHost(uintptr) (*Host, error) { return nil, ErrUnsupported }

func (h *Host) HWND() uintptr                            { return 0 }
func (h *Host) SetBounds(Bounds)                         {}
func (h *Host) Show()                                    {}
func (h *Host) Hide()                                    {}
func (h *Host) Visible() bool                            { return false }
func (h *Host) Close()                                   {}
func (h *Host) ParentClientSize() (int, int, bool)       { return 0, 0, false }

// FindMainWindow is not available on this platform.
func FindMainWindow(time.Duration) (uintptr, error) { return 0, ErrUnsupported }
