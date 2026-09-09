import './styles/theme.css';
import './styles/layout.css';
import './styles/controls.css';
import './styles/panel.css';

import { store } from './state/store';
import type { AppState } from './state/store';
import {
  diagnostics,
  listen,
  handleDrop,
  loadSettings,
  logToBackend,
  refreshPlaylist,
  refreshRecent,
  setFullscreen,
  watchSubtitleTrack,
} from './services/player';
import { OnFileDrop } from '../wailsjs/runtime/runtime';
import { installShortcuts } from './keyboard/shortcuts';
import { TopBar } from './components/TopBar';
import { VideoSurface } from './components/VideoSurface';
import { Drawer } from './components/Drawer';
import { PlayerControls } from './components/PlayerControls';
import { SidePanel } from './components/SidePanel';
import { el, toggleClass } from './util/dom';

/** How long the pointer must be still before fullscreen controls hide. */
const CONTROLS_IDLE_MS = 2500;

/**
 * Assembles the application.
 *
 * The layout is deliberately a column of full-width strips with the video and
 * panel side by side in the middle. Nothing overlaps the video, because the
 * video is a native window that HTML cannot be drawn over; menus and prompts
 * expand the strips around it instead.
 */
function build(): void {
  const root = document.querySelector<HTMLDivElement>('#app');
  if (!root) throw new Error('the #app container is missing from index.html');

  const topBar = new TopBar();
  const videoSurface = new VideoSurface();
  const drawer = new Drawer();
  const controls = new PlayerControls();
  const sidePanel = new SidePanel();

  const stage = el('div', { class: 'stage' }, videoSurface.root, sidePanel.handle, sidePanel.root);
  const shell = el('div', { class: 'shell' }, topBar.root, stage, drawer.root, controls.root);

  root.append(shell);

  topBar.mount();
  videoSurface.mount();
  drawer.mount();
  controls.mount();
  sidePanel.mount();

  installShortcuts();
  installFullscreenBehaviour(shell);
  installFileDrop();
  installDropFeedback(root);

  store.subscribe((state) => applyShellState(shell, state));
}

function applyShellState(shell: HTMLElement, state: AppState): void {
  toggleClass(shell, 'fullscreen', state.fullscreen);
  toggleClass(shell, 'controls-hidden', state.fullscreen && state.controlsHidden);
  toggleClass(shell, 'panel-hidden', !state.panelVisible);
}

/**
 * Hides the control bar after a period of stillness in fullscreen, and brings it
 * back on movement.
 *
 * Hiding grows the video rectangle rather than uncovering anything, so there is
 * no fade and nothing to flicker.
 */
function installFullscreenBehaviour(shell: HTMLElement): void {
  let timer: number | undefined;

  const wake = (): void => {
    const state = store.get();
    if (!state.fullscreen) return;

    if (state.controlsHidden) store.set({ controlsHidden: false });

    window.clearTimeout(timer);
    timer = window.setTimeout(() => {
      const current = store.get();
      // Controls must stay put while a menu is open, or the menu would vanish
      // mid-interaction.
      if (current.fullscreen && current.drawer === 'none') {
        store.set({ controlsHidden: true });
      }
    }, CONTROLS_IDLE_MS);
  };

  window.addEventListener('mousemove', wake);
  window.addEventListener('pointerdown', wake);
  window.addEventListener('keydown', wake);

  // Leaving fullscreen through the window manager rather than the button must
  // still be reflected in the interface.
  document.addEventListener('fullscreenchange', () => {
    const active = document.fullscreenElement !== null;
    if (!active && store.get().fullscreen) setFullscreen(false);
  });

  shell.addEventListener('dblclick', (event) => {
    // Double-clicking the video area toggles fullscreen, as in every other
    // player. Clicks on controls are left alone.
    const target = event.target as HTMLElement;
    if (target.closest('.controls, .side-panel, .drawer, .topbar-wrap')) return;
    setFullscreen(!store.get().fullscreen);
  });
}

/**
 * Accepts dropped files, and shows feedback while one is dragged over.
 *
 * The registration below is not optional decoration: Wails' JavaScript
 * OnFileDrop is what attaches its own dragover/drop listeners to the window.
 * Its Go counterpart only subscribes to the resulting event, so without this
 * call nothing resolves the dropped paths and every drop is silently ignored.
 *
 * Only real filesystem paths are of any use here, and the browser's drop event
 * cannot see them — Wails obtains them from WebView2 and hands them over.
 */
function installFileDrop(): void {
  OnFileDrop((x, y, paths) => {
    logToBackend(`file drop at (${x},${y}): ${paths.join(' | ')}`, 'info');
    if (paths.length > 0) void handleDrop(paths);
  }, true);
}

/** Visual feedback while a file is dragged over the window. */
function installDropFeedback(root: HTMLElement): void {
  let depth = 0;
  let traced = false;

  const setActive = (active: boolean) => toggleClass(root, 'drop-active', active);

  /**
   * Records the state of every precondition Wails requires for a file drop.
   *
   * A drop that is rejected produces no error anywhere: Wails simply returns.
   * This runs once per session at debug level and names which precondition
   * failed, which is the difference between a five-minute diagnosis and an
   * afternoon of guessing.
   */
  const traceDropSupport = (event: DragEvent): void => {
    if (traced) return;
    traced = true;

    const wails = (window as unknown as {
      wails?: { flags?: Record<string, unknown> };
      chrome?: { webview?: { postMessageWithAdditionalObjects?: unknown } };
    }).wails;
    const webview = (window as unknown as {
      chrome?: { webview?: { postMessageWithAdditionalObjects?: unknown } };
    }).chrome?.webview;

    const property = String(wails?.flags?.cssDropProperty ?? '--wails-drop-target');
    const target = event.target instanceof Element ? event.target : null;
    const computed = target ? getComputedStyle(target).getPropertyValue(property).trim() : '(no element)';

    logToBackend(
      [
        'drag support:',
        `enableWailsDragAndDrop=${wails?.flags?.enableWailsDragAndDrop}`,
        `cssDropProperty=${property}`,
        `cssDropValue=${String(wails?.flags?.cssDropValue)}`,
        `computedOnTarget="${computed}"`,
        `target=${target?.tagName ?? '?'}.${target?.className ?? ''}`,
        `canResolveFilePaths=${typeof webview?.postMessageWithAdditionalObjects}`,
        `types=[${Array.from(event.dataTransfer?.types ?? []).join(',')}]`,
      ].join(' '),
      'info',
    );
  };

  window.addEventListener('dragenter', (event) => {
    traceDropSupport(event);
    event.preventDefault();
    depth += 1;
    setActive(true);
  });

  window.addEventListener('dragover', (event) => {
    event.preventDefault();
  });

  window.addEventListener('dragleave', (event) => {
    event.preventDefault();
    depth = Math.max(0, depth - 1);
    if (depth === 0) setActive(false);
  });

  window.addEventListener('drop', (event) => {
    logToBackend(
      `drop event: types=[${Array.from(event.dataTransfer?.types ?? []).join(',')}] ` +
        `files=${event.dataTransfer?.files?.length ?? 0}`,
      'info',
    );
    event.preventDefault();
    depth = 0;
    setActive(false);
  });
}

/**
 * Routes uncaught interface failures into the backend log.
 *
 * A production build has no console to read, so without this a JavaScript
 * exception simply makes the interface stop updating with no trace of why.
 */
function installErrorReporting(): void {
  window.addEventListener('error', (event) => {
    const detail = event.error instanceof Error && event.error.stack
      ? event.error.stack
      : `${event.message} at ${event.filename}:${event.lineno}:${event.colno}`;
    logToBackend(`uncaught error: ${detail}`);
  });

  window.addEventListener('unhandledrejection', (event) => {
    const reason = event.reason;
    const detail = reason instanceof Error && reason.stack ? reason.stack : String(reason);
    logToBackend(`unhandled rejection: ${detail}`);
  });
}

async function start(): Promise<void> {
  installErrorReporting();

  build();
  listen();
  watchSubtitleTrack();

  await loadSettings();
  await refreshRecent();
  await refreshPlaylist();

  // The engine reports readiness through an event, but a page reload during
  // development can miss it, so the state is also read directly.
  const diag = await diagnostics();
  if (diag) {
    store.set({
      diagnostics: diag,
      engineChecked: diag.engineReady || Boolean(diag.startupError),
      engineReady: diag.engineReady,
      startupError: diag.startupError,
    });
  }
}

void start();
