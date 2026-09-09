//go:build windows

// Package winvideo owns the native child window that mpv renders into.
//
// This is the one place in PlayerOne that touches Win32 directly. The window it
// creates is passed to mpv as --wid, which is what puts hardware-accelerated
// video inside the application window instead of in a separate one.
package winvideo

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"playerone/internal/branding"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW        = user32.NewProc("RegisterClassExW")
	procCreateWindowExW         = user32.NewProc("CreateWindowExW")
	procDefWindowProcW          = user32.NewProc("DefWindowProcW")
	procDestroyWindow           = user32.NewProc("DestroyWindow")
	procSetWindowPos            = user32.NewProc("SetWindowPos")
	procShowWindow              = user32.NewProc("ShowWindow")
	procGetMessageW             = user32.NewProc("GetMessageW")
	procTranslateMessage        = user32.NewProc("TranslateMessage")
	procDispatchMessageW        = user32.NewProc("DispatchMessageW")
	procPostMessageW            = user32.NewProc("PostMessageW")
	procEnumWindows             = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procGetClassNameW           = user32.NewProc("GetClassNameW")
	procIsWindowVisible         = user32.NewProc("IsWindowVisible")
	procGetClientRect           = user32.NewProc("GetClientRect")
	procInvalidateRect          = user32.NewProc("InvalidateRect")

	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

// Win32 constants, named as the SDK names them so they can be looked up.
const (
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsClipSiblings = 0x04000000
	wsClipChildren = 0x02000000

	swpNoActivate = 0x0010
	swpNoZOrder   = 0x0004
	swpShowWindow = 0x0040
	swpNoCopyBits = 0x0100

	hwndTop = 0

	swHide           = 0
	swShowNoActivate = 4

	wmDestroy    = 0x0002
	wmClose      = 0x0010
	wmEraseBkgnd = 0x0014

	csHRedraw = 0x0002
	csVRedraw = 0x0001
	csOwnDC   = 0x0020
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type point struct{ X, Y int32 }

type msg struct {
	HWND    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type rect struct{ Left, Top, Right, Bottom int32 }

// Bounds is a rectangle in physical pixels, relative to the parent's client
// area.
type Bounds struct {
	X, Y, Width, Height int
}

// Host is the native child window mpv renders into.
//
// The window lives on its own OS thread with its own message loop. A child
// window's messages are dispatched by the thread that created it, and Wails
// gives no way to run code on its UI thread, so owning a thread is the
// dependable option. Cross-thread SetWindowPos and ShowWindow calls are then
// serviced by that loop, which is exactly what makes them safe from any
// goroutine.
type Host struct {
	parent windows.HWND

	hwnd atomic.Uintptr

	ready   chan error
	stopped chan struct{}

	closeOnce sync.Once

	mu      sync.Mutex
	bounds  Bounds
	visible bool
}

// classOnce guards registering the window class, which must happen exactly once
// per process.
var (
	classOnce sync.Once
	classErr  error
	classAtom uintptr
)

// wndProc handles the few messages the host window cares about.
//
// It is a Go callback invoked from Windows, so it must not panic and must not
// block; everything here is trivial by design.
func wndProc(hwnd windows.HWND, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmEraseBkgnd:
		// The class brush already paints the window black. Claiming the erase
		// prevents the flash of the default background that would otherwise
		// appear between resizing the window and mpv drawing its next frame.
		return 1

	case wmDestroy:
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return ret
}

func registerClass() (uintptr, error) {
	classOnce.Do(func() {
		hInstance, _, _ := procGetModuleHandleW.Call(0)

		// Black, so letterbox bars and the moment before the first frame match
		// the application's dark background instead of flashing white.
		brush, _, _ := procCreateSolidBrush.Call(0x00000000)

		className, err := windows.UTF16PtrFromString(branding.VideoWindowClass)
		if err != nil {
			classErr = fmt.Errorf("winvideo: encoding the class name: %w", err)
			return
		}

		wc := wndClassExW{
			style:         csHRedraw | csVRedraw | csOwnDC,
			lpfnWndProc:   windows.NewCallback(wndProc),
			hInstance:     windows.Handle(hInstance),
			hbrBackground: windows.Handle(brush),
			lpszClassName: className,
		}
		wc.cbSize = uint32(unsafe.Sizeof(wc))

		atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if atom == 0 {
			classErr = fmt.Errorf("winvideo: registering the window class: %w", callErr)
			return
		}
		classAtom = atom
	})

	return classAtom, classErr
}

// NewHost creates the video window as a child of parent and starts its message
// loop. It returns once the window exists and its handle is available.
func NewHost(parent uintptr) (*Host, error) {
	if parent == 0 {
		return nil, fmt.Errorf("winvideo: no parent window was given")
	}

	h := &Host{
		parent:  windows.HWND(parent),
		ready:   make(chan error, 1),
		stopped: make(chan struct{}),
	}

	go h.run()

	if err := <-h.ready; err != nil {
		return nil, err
	}
	return h, nil
}

// run owns the window for its whole lifetime.
func (h *Host) run() {
	// The window and its message loop must stay on one OS thread; without this
	// the Go scheduler could move the loop and messages would stop arriving.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(h.stopped)

	if _, err := registerClass(); err != nil {
		h.ready <- err
		return
	}

	className, err := windows.UTF16PtrFromString(branding.VideoWindowClass)
	if err != nil {
		h.ready <- fmt.Errorf("winvideo: encoding the class name: %w", err)
		return
	}
	windowName, _ := windows.UTF16PtrFromString("")

	hInstance, _, _ := procGetModuleHandleW.Call(0)

	// Created hidden: the empty state must be visible until a file is opened,
	// and a native window cannot be covered by HTML.
	hwnd, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		wsChild|wsClipSiblings|wsClipChildren,
		0, 0, 1, 1,
		uintptr(h.parent),
		0,
		hInstance,
		0,
	)
	if hwnd == 0 {
		h.ready <- fmt.Errorf("winvideo: creating the video window: %w", callErr)
		return
	}

	h.hwnd.Store(hwnd)
	h.ready <- nil

	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)

		// GetMessage returns 0 for WM_QUIT and -1 for an error; both end the
		// loop, since continuing after an error would spin forever.
		if int32(ret) <= 0 {
			return
		}

		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// HWND is the handle to pass to mpv as --wid.
func (h *Host) HWND() uintptr { return h.hwnd.Load() }

// SetBounds moves and resizes the video window within its parent.
//
// Coordinates are physical pixels relative to the parent's client area, which is
// what the frontend produces by scaling its CSS rectangle by the device pixel
// ratio.
func (h *Host) SetBounds(b Bounds) {
	hwnd := h.hwnd.Load()
	if hwnd == 0 {
		return
	}

	if b.Width < 0 {
		b.Width = 0
	}
	if b.Height < 0 {
		b.Height = 0
	}

	h.mu.Lock()
	unchanged := h.bounds == b
	h.bounds = b
	h.mu.Unlock()

	if unchanged {
		return // resize storms during a drag would otherwise repaint constantly
	}

	// HWND_TOP keeps the video above the WebView2 child window. Without it the
	// z-order after a WebView2 relayout is not guaranteed and the video can
	// disappear behind the page.
	procSetWindowPos.Call(
		hwnd,
		hwndTop,
		uintptr(int32(b.X)),
		uintptr(int32(b.Y)),
		uintptr(int32(b.Width)),
		uintptr(int32(b.Height)),
		swpNoActivate|swpNoCopyBits,
	)
}

// Show makes the video window visible.
func (h *Host) Show() {
	hwnd := h.hwnd.Load()
	if hwnd == 0 {
		return
	}

	h.mu.Lock()
	already := h.visible
	h.visible = true
	h.mu.Unlock()

	if already {
		return
	}

	// SW_SHOWNOACTIVATE rather than SW_SHOW: the video window must never take
	// focus, or keyboard shortcuts would stop reaching the WebView.
	procShowWindow.Call(hwnd, swShowNoActivate)
	procSetWindowPos.Call(hwnd, hwndTop, 0, 0, 0, 0, swpNoActivate|0x0001|0x0002) // SWP_NOSIZE|SWP_NOMOVE
}

// Hide removes the video window so the HTML underneath becomes visible. This is
// how the empty state, the error screen and the resume prompt are shown despite
// a native window sitting on top of the page.
func (h *Host) Hide() {
	hwnd := h.hwnd.Load()
	if hwnd == 0 {
		return
	}

	h.mu.Lock()
	already := !h.visible
	h.visible = false
	h.mu.Unlock()

	if already {
		return
	}

	procShowWindow.Call(hwnd, swHide)

	// The page underneath was never painted while it was covered, so ask for it
	// to be redrawn now that it is exposed.
	procInvalidateRect.Call(uintptr(h.parent), 0, 1)
}

// Visible reports whether the video window is currently shown.
func (h *Host) Visible() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.visible
}

// Close destroys the window and stops its message loop.
func (h *Host) Close() {
	h.closeOnce.Do(func() {
		hwnd := h.hwnd.Load()
		if hwnd == 0 {
			return
		}

		// Posting rather than calling DestroyWindow directly: a window may only
		// be destroyed by the thread that created it.
		procPostMessageW.Call(hwnd, wmClose, 0, 0)

		select {
		case <-h.stopped:
		case <-time.After(2 * time.Second):
			// The loop is wedged. Destroying from here is not strictly legal,
			// but the process is exiting and leaking the window would be worse.
			procDestroyWindow.Call(hwnd)
		}
		h.hwnd.Store(0)
	})
}

// ParentClientSize returns the parent window's client area in physical pixels.
// Fullscreen sizing uses it so the video can fill the window exactly.
func (h *Host) ParentClientSize() (width, height int, ok bool) {
	var r rect
	ret, _, _ := procGetClientRect.Call(uintptr(h.parent), uintptr(unsafe.Pointer(&r)))
	if ret == 0 {
		return 0, 0, false
	}
	return int(r.Right - r.Left), int(r.Bottom - r.Top), true
}

// FindMainWindow locates this process's Wails window.
//
// Wails v2 offers no way to ask for its HWND, so the window is found by class
// name and owning process. Both conditions are needed: the class alone would
// also match a second copy of PlayerOne, or any other Wails application.
//
// It polls because the window is created during wails.Run, which may not have
// happened yet when startup code runs.
func FindMainWindow(timeout time.Duration) (uintptr, error) {
	const wailsClass = "wailsWindow"

	deadline := time.Now().Add(timeout)
	for {
		if hwnd := findWindowByClass(wailsClass, windows.GetCurrentProcessId()); hwnd != 0 {
			return hwnd, nil
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("winvideo: could not find the application window (class %q) within %s", wailsClass, timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func findWindowByClass(class string, pid uint32) uintptr {
	var found uintptr

	callback := windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var windowPID uint32
		procGetWindowThreadProcessID.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&windowPID)))
		if windowPID != pid {
			return 1 // keep enumerating
		}

		buf := make([]uint16, 256)
		n, _, _ := procGetClassNameW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if n == 0 || windows.UTF16ToString(buf[:n]) != class {
			return 1
		}

		visible, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
		if visible == 0 {
			// Wails creates the window before showing it; an invisible match is
			// still the right window, but a visible one is preferred if the
			// enumeration finds both.
			if found == 0 {
				found = uintptr(hwnd)
			}
			return 1
		}

		found = uintptr(hwnd)
		return 0 // stop: this is the one
	})

	procEnumWindows.Call(callback, 0)
	return found
}
