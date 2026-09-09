import './styles/theme.css';
import './styles/layout.css';
import './styles/controls.css';
import './styles/panel.css';

import { store } from './state/store';
import type { AppState } from './state/store';
import {
  diagnostics,
  listen,
  loadSettings,
  logToBackend,
  refreshPlaylist,
  refreshRecent,
  setFullscreen,
  watchSubtitleTrack,
} from './services/player';
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
 * Visual feedback while a file is dragged over the window.
 *
 * The drop itself is handled natively by Wails, which reports the real
 * filesystem paths; the browser's own drop event cannot see those.
 */
function installDropFeedback(root: HTMLElement): void {
  let depth = 0;

  const setActive = (active: boolean) => toggleClass(root, 'drop-active', active);

  window.addEventListener('dragenter', (event) => {
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
