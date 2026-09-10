// Command PlayerOne is a local video player for tutorial and course material.
//
// It pairs mpv's decoding with a YouTube-style chapter and transcript panel, so
// a downloaded lecture can be navigated the way an online one can.
package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"playerone/internal/branding"
	"playerone/internal/settings"
	"playerone/internal/version"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	app := NewApp()

	// The window geometry is read before Wails starts, since the options are
	// fixed at creation time. A failure here is not worth reporting: the
	// defaults produce a perfectly good window.
	geometry := savedWindowGeometry()

	err := wails.Run(&options.App{
		Title:  windowTitle(),
		Width:  geometry.Width,
		Height: geometry.Height,
		MinWidth:  900,
		MinHeight: 560,

		// The page is black behind the video slot, so the moment before mpv
		// draws its first frame matches the interface instead of flashing.
		BackgroundColour: &options.RGBA{R: 15, G: 15, B: 15, A: 1},

		AssetServer: &assetserver.Options{Assets: assets},

		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},

		Windows: &windows.Options{
			// The video is a native child window, so WebView2 must not be
			// transparent or composited in ways that fight with it.
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			// Right-clicking the video should not offer WebView2's developer
			// menu in a shipped build.
			DisableWindowIcon: false,
		},

		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			restoreWindowPosition(ctx, geometry)
			openCommandLineTarget(app)
		},
		OnDomReady:  app.domReady,
		OnBeforeClose: func(ctx context.Context) bool {
			saveWindowGeometry(ctx, app)
			return false // never veto the close
		},
		OnShutdown: app.shutdown,

		Bind: []any{app},
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s failed to start: %v\n", branding.Name, err)
		os.Exit(1)
	}
}

// windowTitle names the window, including the version for a release build.
//
// The title bar is the one place the version is visible without opening
// anything, which is what makes "which version are you running?" answerable in
// a screenshot. A development build says so rather than showing a number it
// does not have.
func windowTitle() string {
	if version.IsRelease() {
		return branding.Name + " v" + version.Version
	}
	return branding.Name + " (development build)"
}

// savedWindowGeometry reads the remembered window size, falling back to the
// defaults when nothing has been stored.
func savedWindowGeometry() settings.WindowState {
	defaults := settings.Defaults().Window

	store, err := settings.DefaultStore()
	if err != nil {
		return defaults
	}

	saved := store.Get().Window
	if !saved.Valid || saved.Width <= 0 || saved.Height <= 0 {
		return defaults
	}
	return saved
}

// restoreWindowPosition applies the remembered position once the window exists.
//
// Position cannot be set through the options, and a window restored to a screen
// that is no longer connected would be invisible, so an off-screen position is
// discarded in favour of centring.
func restoreWindowPosition(ctx context.Context, geometry settings.WindowState) {
	if !geometry.Valid {
		wailsruntime.WindowCenter(ctx)
		return
	}

	if geometry.Maximised {
		wailsruntime.WindowMaximise(ctx)
		return
	}

	if isOnAScreen(ctx, geometry) {
		wailsruntime.WindowSetPosition(ctx, geometry.X, geometry.Y)
	} else {
		wailsruntime.WindowCenter(ctx)
	}
}

// isOnAScreen reports whether a saved position still falls on a connected
// display, so unplugging a monitor cannot strand the window off-screen.
func isOnAScreen(ctx context.Context, geometry settings.WindowState) bool {
	screens, err := wailsruntime.ScreenGetAll(ctx)
	if err != nil || len(screens) == 0 {
		return false
	}

	// A window counts as visible if its title bar area overlaps a screen; that
	// is enough for the user to drag it back.
	const grabbableHeight = 40
	for _, s := range screens {
		if geometry.X+geometry.Width > 0 && geometry.X < s.Width &&
			geometry.Y+grabbableHeight > 0 && geometry.Y < s.Height {
			return true
		}
	}
	return false
}

// saveWindowGeometry records the window's size and position at close.
func saveWindowGeometry(ctx context.Context, app *App) {
	if app.settings == nil {
		return
	}

	maximised := wailsruntime.WindowIsMaximised(ctx)

	// A maximised window reports the screen's size, which would be restored as
	// a non-maximised window filling the screen. Keeping the previous size and
	// only flipping the flag restores correctly.
	if maximised {
		app.persist(func(s *settingsMutation) {
			s.Window.Maximised = true
			s.Window.Valid = true
		})
		return
	}

	width, height := wailsruntime.WindowGetSize(ctx)
	x, y := wailsruntime.WindowGetPosition(ctx)
	app.SaveWindowState(width, height, x, y, false)
}

// openCommandLineTarget opens a file or folder given on the command line, which
// is what makes "Open with PlayerOne" work from Explorer.
//
// Drops are not handled here. Wails delivers them to the interface, which calls
// HandleDrop; subscribing on this side as well would open every dropped file
// twice.
func openCommandLineTarget(app *App) {
	if path := commandLineTarget(); path != "" {
		go openWhenReady(app, path)
	}
}

// commandLineTarget returns the file or folder given on the command line.
//
// Folders are accepted as well as files: dropping a course folder onto the
// executable, or using "Open with" on one, should queue its lessons rather than
// being rejected.
func commandLineTarget() string {
	for _, arg := range os.Args[1:] {
		if arg == "" || arg[0] == '-' {
			continue
		}
		if abs, err := filepath.Abs(arg); err == nil {
			if _, err := os.Stat(abs); err == nil {
				return abs
			}
		}
	}
	return ""
}

// openWhenReady waits for the engine before opening a file from the command
// line, since mpv starts asynchronously.
func openWhenReady(app *App, path string) {
	deadline := time.Now().Add(20 * time.Second)

	for time.Now().Before(deadline) {
		if app.currentEngine() == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		info, err := os.Stat(path)
		if err != nil {
			app.log.Error("app: %s could not be opened from the command line: %v", path, err)
			return
		}

		if info.IsDir() {
			if _, err := app.ReplacePlaylist([]string{path}); err != nil {
				app.log.Error("app: could not queue %s: %v", path, err)
				app.emit(eventError, err.Error())
			}
			return
		}

		if _, err := app.Open(path); err != nil {
			app.log.Error("app: could not open %s from the command line: %v", path, err)
			app.emit(eventError, err.Error())
			return
		}
		_ = app.Play()
		return
	}

	app.log.Warn("app: the engine did not start in time to open %s", path)
}
