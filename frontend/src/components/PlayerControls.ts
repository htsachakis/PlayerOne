import { store } from '../state/store';
import type { AppState, Drawer } from '../state/store';
import {
  cycleRepeat,
  nextTrack,
  persistSidePanel,
  playPause,
  previousTrack,
  seekRelative,
  setFullscreen,
  setRepeat,
  setShuffle,
  setVolume,
  toggleMute,
  toggleReverse,
} from '../services/player';
import { el, setText, toggleClass } from '../util/dom';
import { formatSpeed } from '../util/time';
import { Timeline } from './Timeline';
import { icon } from './icons';
import { TRACK_AUDIO, TRACK_SUBTITLE, NO_TRACK } from '../types/media';

/**
 * The control bar beneath the video.
 *
 * Controls sit below the video rather than floating over it because the video
 * is a native window that HTML cannot be drawn on top of. Menus open as a
 * drawer above this bar, which shrinks the video rather than covering it.
 */
export class PlayerControls {
  readonly root: HTMLElement;

  private readonly timeline = new Timeline();

  private readonly playButton: HTMLButtonElement;
  private readonly reverseButton: HTMLButtonElement;
  private readonly prevButton: HTMLButtonElement;
  private readonly nextButton: HTMLButtonElement;
  private readonly shuffleButton: HTMLButtonElement;
  private readonly repeatButton: HTMLButtonElement;
  private readonly muteButton: HTMLButtonElement;
  private readonly volumeSlider: HTMLInputElement;
  private readonly subtitleButton: HTMLButtonElement;
  private readonly audioButton: HTMLButtonElement;
  private readonly speedButton: HTMLButtonElement;
  private readonly settingsButton: HTMLButtonElement;
  private readonly panelButton: HTMLButtonElement;
  private readonly fullscreenButton: HTMLButtonElement;

  private volumeIsBeingDragged = false;

  constructor() {
    this.playButton = button('play', 'Play / pause  (Space)', () => void playPause());
    this.reverseButton = button('reverse', 'Play backwards  (R)', () => void toggleReverse());

    this.prevButton = button('prevTrack', 'Previous in the playlist', () => void previousTrack());
    this.nextButton = button('nextTrack', 'Next in the playlist', () => void nextTrack());

    this.shuffleButton = button('shuffle', 'Shuffle the playlist', () => {
      void setShuffle(!store.get().playlist.shuffle);
    });
    this.repeatButton = button('repeat', 'Repeat', () => {
      void setRepeat(cycleRepeat(store.get().playlist.repeat));
    });

    const back = button('back10', 'Back 5 seconds  (Left arrow)', () => void seekRelative(-5));
    const forward = button('forward10', 'Forward 5 seconds  (Right arrow)', () => void seekRelative(5));

    this.muteButton = button('volume', 'Mute  (M)', () => void toggleMute());

    this.volumeSlider = el('input', {
      class: 'volume-slider',
      type: 'range',
      min: '0',
      max: '150',
      step: '1',
      value: '100',
      'aria-label': 'Volume',
      title: 'Volume  (Up / Down arrow)',
    }) as HTMLInputElement;

    this.volumeSlider.addEventListener('input', () => {
      this.volumeIsBeingDragged = true;
      void setVolume(Number(this.volumeSlider.value));
    });
    this.volumeSlider.addEventListener('change', () => {
      this.volumeIsBeingDragged = false;
    });

    this.subtitleButton = labelledButton('cc', 'Subtitles', 'Subtitles  (C toggles them off and on)', () =>
      this.toggleDrawer('subtitles'),
    );
    this.audioButton = labelledButton('audio', 'Audio', 'Audio track', () => this.toggleDrawer('audio'));
    this.speedButton = labelledButton('speed', '1x', 'Playback speed  ( [ and ] )', () =>
      this.toggleDrawer('speed'),
    );

    this.settingsButton = button('settings', 'Settings', () => this.toggleDrawer('settings'));
    this.panelButton = button('panel', 'Show or hide the side panel  (P)', () => this.togglePanel());
    this.fullscreenButton = button('fullscreen', 'Fullscreen  (F)', () => this.toggleFullscreen());

    this.root = el(
      'div',
      { class: 'controls' },
      this.timeline.root,
      el(
        'div',
        { class: 'controls-row' },
        el(
          'div',
          { class: 'controls-group controls-left' },
          this.prevButton,
          this.playButton,
          this.nextButton,
          back,
          forward,
          this.reverseButton,
          el('div', { class: 'volume' }, this.muteButton, this.volumeSlider),
        ),
        el(
          'div',
          { class: 'controls-group controls-right' },
          this.shuffleButton,
          this.repeatButton,
          this.subtitleButton,
          this.audioButton,
          this.speedButton,
          this.settingsButton,
          this.panelButton,
          this.fullscreenButton,
        ),
      ),
    );
  }

  mount(): void {
    this.timeline.mount();
    store.subscribe((state) => this.render(state));
  }

  private toggleDrawer(drawer: Drawer): void {
    store.update((state) => ({ drawer: state.drawer === drawer ? 'none' : drawer }));
  }

  private togglePanel(): void {
    store.update((state) => {
      const visible = !state.panelVisible;
      persistSidePanel(visible, state.panelWidth);
      return { panelVisible: visible };
    });
  }

  private toggleFullscreen(): void {
    setFullscreen(!store.get().fullscreen);
  }

  private render(state: AppState): void {
    const { playback } = state;
    const hasFile = playback.fileLoaded;

    setIcon(this.playButton, playback.paused ? 'play' : 'pause');
    this.playButton.title = playback.paused ? 'Play  (Space)' : 'Pause  (Space)';
    this.playButton.disabled = !hasFile;

    // The button always offers the opposite of the current direction, and its
    // icon shows what pressing it will do rather than what is happening now.
    setIcon(this.reverseButton, playback.reverse ? 'forward' : 'reverse');
    toggleClass(this.reverseButton, 'active', playback.reverse);
    this.reverseButton.title = playback.reverse
      ? 'Playing backwards — click to play forwards  (R)'
      : 'Play backwards  (R)';
    this.reverseButton.disabled = !hasFile;

    // Track buttons are only meaningful with a queue, and a disabled button is
    // clearer than one that silently does nothing.
    const queued = state.playlist.items.length;
    this.prevButton.disabled = queued < 1;
    this.nextButton.disabled = queued < 2;
    this.prevButton.title = queued > 1
      ? 'Previous in the playlist  (restarts this file first)'
      : 'Restart this file';

    toggleClass(this.shuffleButton, 'active', state.playlist.shuffle);
    this.shuffleButton.title = state.playlist.shuffle
      ? 'Shuffle is on — click to play in order'
      : 'Shuffle the playlist';
    this.shuffleButton.disabled = queued < 2;

    setIcon(this.repeatButton, state.playlist.repeat === 'one' ? 'repeatOne' : 'repeat');
    toggleClass(this.repeatButton, 'active', state.playlist.repeat !== 'off');
    this.repeatButton.title = repeatTitle(state.playlist.repeat);

    setIcon(this.muteButton, playback.muted || playback.volume === 0 ? 'volumeOff' : 'volume');
    this.muteButton.title = playback.muted ? 'Unmute  (M)' : 'Mute  (M)';

    if (!this.volumeIsBeingDragged) {
      const shown = playback.muted ? 0 : Math.round(playback.volume);
      if (this.volumeSlider.value !== String(shown)) this.volumeSlider.value = String(shown);
    }

    setLabel(this.subtitleButton, subtitleLabel(state));
    toggleClass(this.subtitleButton, 'active', playback.subtitleId !== NO_TRACK);
    toggleClass(this.subtitleButton, 'open', state.drawer === 'subtitles');
    this.subtitleButton.disabled = !hasFile;

    setLabel(this.audioButton, audioLabel(state));
    toggleClass(this.audioButton, 'open', state.drawer === 'audio');
    this.audioButton.disabled = !hasFile;

    setLabel(this.speedButton, formatSpeed(playback.speed));
    toggleClass(this.speedButton, 'active', playback.speed !== 1);
    toggleClass(this.speedButton, 'open', state.drawer === 'speed');
    this.speedButton.disabled = !hasFile;

    toggleClass(this.settingsButton, 'open', state.drawer === 'settings');
    toggleClass(this.panelButton, 'active', state.panelVisible);
    setIcon(this.fullscreenButton, state.fullscreen ? 'exitFullscreen' : 'fullscreen');
    this.fullscreenButton.title = state.fullscreen ? 'Leave fullscreen  (Esc)' : 'Fullscreen  (F)';
  }
}

/** Names the selected subtitle track, or says subtitles are off. */
function subtitleLabel(state: AppState): string {
  if (state.playback.subtitleId === NO_TRACK) return 'Subtitles off';

  const track = state.tracks.find(
    (t) => t.type === TRACK_SUBTITLE && t.id === state.playback.subtitleId,
  );
  if (!track) return 'Subtitles';

  // The full label is long ("English — SRT"); the button shows only the name.
  return shortName(track.label);
}

function audioLabel(state: AppState): string {
  const track = state.tracks.find((t) => t.type === TRACK_AUDIO && t.id === state.playback.audioId);
  if (!track) return 'Audio';
  return shortName(track.label);
}

function repeatTitle(mode: string): string {
  switch (mode) {
    case 'all':
      return 'Repeat the whole playlist — click to repeat one';
    case 'one':
      return 'Repeat this file — click to turn repeat off';
    default:
      return 'Repeat is off — click to repeat the playlist';
  }
}

function shortName(label: string): string {
  const dash = label.indexOf(' — ');
  return dash > 0 ? label.slice(0, dash) : label;
}

function button(name: string, title: string, onClick: () => void): HTMLButtonElement {
  const node = el('button', { class: 'ctl-button', type: 'button', title, 'aria-label': title });
  node.append(icon(name));
  node.addEventListener('click', onClick);
  return node as HTMLButtonElement;
}

function labelledButton(
  name: string,
  label: string,
  title: string,
  onClick: () => void,
): HTMLButtonElement {
  const node = el('button', { class: 'ctl-button ctl-labelled', type: 'button', title });
  node.append(icon(name), el('span', { class: 'ctl-label' }, label));
  node.addEventListener('click', onClick);
  return node as HTMLButtonElement;
}

function setIcon(node: HTMLElement, name: string): void {
  const existing = node.querySelector('svg');
  if (existing?.dataset.icon === name) return;

  const next = icon(name);
  if (existing) existing.replaceWith(next);
  else node.prepend(next);
}

function setLabel(node: HTMLElement, text: string): void {
  const label = node.querySelector('.ctl-label');
  if (label) setText(label, text);
}
