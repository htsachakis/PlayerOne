import { store } from '../state/store';
import {
  chooseAndOpen,
  clearError,
  nextTrack,
  previousTrack,
  disableSubtitles,
  persistSidePanel,
  playPause,
  seekRelative,
  setFullscreen,
  setSpeed,
  setSubtitleTrack,
  setVolume,
  toggleMute,
  toggleReverse,
} from '../services/player';
import { NO_TRACK, TRACK_SUBTITLE } from '../types/media';

/** The speed ladder the bracket keys step through. */
const SPEED_LADDER = [0.25, 0.5, 0.75, 1.0, 1.25, 1.5, 1.75, 2.0, 3.0, 4.0, 8.0];

/**
 * Global keyboard shortcuts.
 *
 * mpv's own key handling is disabled on the backend, so every shortcut is
 * handled exactly once, here, and cannot double-fire when the video window
 * happens to have focus.
 */
export function installShortcuts(): void {
  window.addEventListener('keydown', onKeyDown);
}

function onKeyDown(event: KeyboardEvent): void {
  // Typing in the transcript search box must produce text, not seek the video.
  if (isTyping(event.target)) {
    if (event.key === 'Escape') (event.target as HTMLElement).blur();
    return;
  }

  // Leave the browser's own accelerators alone, except for the ones claimed
  // explicitly below.
  if (event.altKey || event.metaKey) return;

  const state = store.get();

  if (event.ctrlKey) {
    switch (event.key) {
      case 'o':
      case 'O':
        event.preventDefault();
        void chooseAndOpen();
        break;

      // Three seek distances on the same keys: 5 seconds bare, 30 with Shift,
      // 60 with Ctrl. Skipping a minute of preamble is a single keypress.
      case 'ArrowLeft':
        event.preventDefault();
        void seekRelative(-60);
        break;

      case 'ArrowRight':
        event.preventDefault();
        void seekRelative(60);
        break;

      default:
        break;
    }
    return;
  }

  switch (event.key) {
    case ' ':
    case 'Spacebar':
      event.preventDefault();
      void playPause();
      break;

    case 'ArrowLeft':
      event.preventDefault();
      void seekRelative(event.shiftKey ? -30 : -5);
      break;

    case 'ArrowRight':
      event.preventDefault();
      void seekRelative(event.shiftKey ? 30 : 5);
      break;

    case 'ArrowUp':
      event.preventDefault();
      void setVolume(Math.min(150, state.playback.volume + 5));
      break;

    case 'ArrowDown':
      event.preventDefault();
      void setVolume(Math.max(0, state.playback.volume - 5));
      break;

    case 'm':
    case 'M':
      event.preventDefault();
      void toggleMute();
      break;

    case 'f':
    case 'F':
      event.preventDefault();
      setFullscreen(!state.fullscreen);
      break;

    case 'c':
    case 'C':
      event.preventDefault();
      toggleSubtitles();
      break;

    case 'r':
    case 'R':
      event.preventDefault();
      void toggleReverse();
      break;

    case 'p':
    case 'P':
      event.preventDefault();
      togglePanel();
      break;

    case 'n':
    case 'N':
      event.preventDefault();
      void nextTrack();
      break;

    case 'b':
    case 'B':
      event.preventDefault();
      void previousTrack();
      break;

    case '[':
      event.preventDefault();
      void setSpeed(stepSpeed(state.playback.speed, -1));
      break;

    case ']':
      event.preventDefault();
      void setSpeed(stepSpeed(state.playback.speed, 1));
      break;

    case '0':
      event.preventDefault();
      void setSpeed(1);
      break;

    case 'Escape':
      event.preventDefault();
      onEscape();
      break;

    default:
      break;
  }
}

/**
 * Escape closes whatever is temporarily open, one layer at a time, before
 * leaving fullscreen. Closing everything at once would be surprising.
 */
function onEscape(): void {
  const state = store.get();

  if (state.drawer !== 'none' && state.drawer !== 'resume') {
    store.set({ drawer: 'none' });
    return;
  }
  if (state.error) {
    clearError();
    return;
  }
  if (state.fullscreen) {
    setFullscreen(false);
  }
}

/**
 * Toggles subtitles between off and the last text track.
 *
 * Turning them back on prefers the previously selected track, falling back to
 * the first text track, because restoring an image-based track would leave the
 * transcript unavailable for no reason the user could see.
 */
let lastSubtitleTrack = NO_TRACK;

function toggleSubtitles(): void {
  const state = store.get();
  const current = state.playback.subtitleId;

  if (current !== NO_TRACK) {
    lastSubtitleTrack = current;
    void disableSubtitles();
    return;
  }

  const tracks = state.tracks.filter((t) => t.type === TRACK_SUBTITLE);
  if (tracks.length === 0) return;

  const remembered = tracks.find((t) => t.id === lastSubtitleTrack);
  const preferred = remembered ?? tracks.find((t) => !t.imageBased) ?? tracks[0];
  void setSubtitleTrack(preferred.id);
}

function togglePanel(): void {
  store.update((state) => {
    const visible = !state.panelVisible;
    persistSidePanel(visible, state.panelWidth);
    return { panelVisible: visible };
  });
}

/** Moves one step along the speed ladder from wherever playback currently is. */
function stepSpeed(current: number, direction: number): number {
  const index = SPEED_LADDER.findIndex((s) => Math.abs(s - current) < 0.001);

  if (index >= 0) {
    const next = index + direction;
    if (next < 0) return SPEED_LADDER[0];
    if (next >= SPEED_LADDER.length) return SPEED_LADDER[SPEED_LADDER.length - 1];
    return SPEED_LADDER[next];
  }

  // The speed was set to something off the ladder; move to the nearest rung in
  // the requested direction.
  if (direction > 0) return SPEED_LADDER.find((s) => s > current) ?? SPEED_LADDER[SPEED_LADDER.length - 1];
  return [...SPEED_LADDER].reverse().find((s) => s < current) ?? SPEED_LADDER[0];
}

function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;

  const tag = target.tagName;
  return (
    tag === 'INPUT' ||
    tag === 'TEXTAREA' ||
    tag === 'SELECT' ||
    target.isContentEditable
  );
}
