import { store } from '../state/store';
import type { AppState } from '../state/store';
import {
  acceptResume,
  checkForUpdates,
  setCheckForUpdates,
  chooseAndAddSubtitle,
  clearRecent,
  declineResume,
  disableSubtitles,
  forgetRecent,
  openPath,
  saveSettings,
  setAudioDelay,
  setAudioTrack,
  setAutoResume,
  setSpeed,
  setSubtitleDelay,
  setSubtitleScale,
  setSubtitleTrack,
  SUBTITLE_SCALE_MAX,
  SUBTITLE_SCALE_MIN,
} from '../services/player';
import { clear, el } from '../util/dom';
import { formatSpeed, formatTime } from '../util/time';
import { icon } from './icons';
import { NO_TRACK, TRACK_AUDIO, TRACK_SUBTITLE } from '../types/media';

/** The playback rates offered, including the fast skim speeds. */
const SPEED_PRESETS = [0.25, 0.5, 0.75, 1.0, 1.25, 1.5, 1.75, 2.0, 3.0, 4.0, 8.0];

/**
 * The expanding panel between the video and the control bar.
 *
 * Menus live here rather than floating over the video because the video is a
 * native window that HTML cannot cover. Opening a menu shrinks the video area
 * instead, which the video surface picks up through its ResizeObserver.
 */
export class Drawer {
  readonly root: HTMLElement;
  private readonly body: HTMLElement;
  private rendered: string = '';

  constructor() {
    this.body = el('div', { class: 'drawer-body' });
    this.root = el('div', { class: 'drawer', hidden: true }, this.body);
  }

  mount(): void {
    store.subscribe((state) => this.render(state));
  }

  private render(state: AppState): void {
    const open = state.drawer !== 'none';
    this.root.hidden = !open;

    if (!open) {
      this.rendered = '';
      clear(this.body);
      return;
    }

    // Re-rendering the drawer on every playback tick would fight with clicks,
    // so it is rebuilt only when something it displays actually changes.
    const signature = this.signature(state);
    if (signature === this.rendered) return;
    this.rendered = signature;

    clear(this.body);
    this.body.append(this.buildContent(state));
  }

  private signature(state: AppState): string {
    switch (state.drawer) {
      case 'subtitles':
        return `sub|${state.playback.subtitleId}|${state.tracks.length}|` +
          `${state.playback.subtitleDelay}|${state.playback.subtitleScale}`;
      case 'audio':
        return `aud|${state.playback.audioId}|${state.tracks.length}|${state.playback.audioDelay}`;
      case 'speed':
        return `spd|${state.playback.speed}`;
      case 'settings':
        return `set|${JSON.stringify(state.settings)}|${state.update?.version ?? ''}`;
      case 'recent':
        return `rec|${state.recent.map((r) => r.path + r.position).join('|')}`;
      case 'resume':
        return `res|${state.resumePrompt?.position ?? ''}|${state.settings.autoResume}`;
      default:
        return 'none';
    }
  }

  private buildContent(state: AppState): HTMLElement {
    switch (state.drawer) {
      case 'subtitles':
        return this.subtitleMenu(state);
      case 'audio':
        return this.audioMenu(state);
      case 'speed':
        return this.speedMenu(state);
      case 'settings':
        return this.settingsMenu(state);
      case 'recent':
        return this.recentMenu(state);
      case 'resume':
        return this.resumePrompt(state);
      default:
        return el('div');
    }
  }

  // --- Subtitles ---

  private subtitleMenu(state: AppState): HTMLElement {
    const tracks = state.tracks.filter((t) => t.type === TRACK_SUBTITLE);
    const current = state.playback.subtitleId;

    const options = el('div', { class: 'menu-options' });

    options.append(
      option('Off', current === NO_TRACK, () => {
        void disableSubtitles();
        close();
      }),
    );

    for (const track of tracks) {
      options.append(
        option(track.label, current === track.id, () => {
          void setSubtitleTrack(track.id);
          close();
        }),
      );
    }

    if (tracks.length === 0) {
      options.append(el('p', { class: 'menu-empty' }, 'This video has no subtitle tracks.'));
    }

    return el(
      'div',
      { class: 'menu' },
      menuHeader('Subtitles'),
      options,
      el(
        'div',
        { class: 'menu-footer' },
        actionButton('subtitleFile', 'Load subtitle file…', () => {
          void chooseAndAddSubtitle();
          close();
        }),
        scaleControl('Subtitle size', state.playback.subtitleScale, (value) => {
          void setSubtitleScale(value);
        }),
        delayControl('Subtitle delay', state.playback.subtitleDelay, (value) => {
          void setSubtitleDelay(value);
        }),
      ),
    );
  }

  // --- Audio ---

  private audioMenu(state: AppState): HTMLElement {
    const tracks = state.tracks.filter((t) => t.type === TRACK_AUDIO);
    const current = state.playback.audioId;

    const options = el('div', { class: 'menu-options' });

    for (const track of tracks) {
      options.append(
        option(track.label, current === track.id, () => {
          void setAudioTrack(track.id);
          close();
        }),
      );
    }

    if (tracks.length === 0) {
      options.append(el('p', { class: 'menu-empty' }, 'This video has no audio tracks.'));
    } else if (tracks.length > 1) {
      options.append(
        option('Off', current === NO_TRACK, () => {
          void setAudioTrack(NO_TRACK);
          close();
        }),
      );
    }

    return el(
      'div',
      { class: 'menu' },
      menuHeader('Audio track'),
      options,
      el(
        'div',
        { class: 'menu-footer' },
        delayControl('Audio delay', state.playback.audioDelay, (value) => {
          void setAudioDelay(value);
        }),
      ),
    );
  }

  // --- Speed ---

  private speedMenu(state: AppState): HTMLElement {
    const current = state.playback.speed;
    const options = el('div', { class: 'menu-options menu-grid' });

    for (const speed of SPEED_PRESETS) {
      options.append(
        option(speed === 1 ? 'Normal' : formatSpeed(speed), Math.abs(current - speed) < 0.001, () => {
          void setSpeed(speed);
          close();
        }),
      );
    }

    return el(
      'div',
      { class: 'menu' },
      menuHeader('Playback speed'),
      options,
      el(
        'p',
        { class: 'menu-note' },
        'Pitch is corrected automatically, so speech stays intelligible at high speeds. ' +
          'Press R to play backwards; reverse playback is demanding and may stutter on long files.',
      ),
    );
  }

  // --- Settings ---

  private settingsMenu(state: AppState): HTMLElement {
    const settings = state.settings;

    return el(
      'div',
      { class: 'menu' },
      menuHeader('Settings'),
      el(
        'div',
        { class: 'settings-list' },
        checkbox('Always resume automatically', settings.autoResume, (checked) => {
          setAutoResume(checked);
        }),
        checkbox('Transcript follows playback', settings.followTranscript, (checked) => {
          void saveSettings({ ...settings, followTranscript: checked });
          store.set({ followTranscript: checked });
        }),
        el(
          'label',
          { class: 'settings-row' },
          el('span', {}, 'Log detail'),
          selectBox(['INFO', 'DEBUG', 'WARN', 'ERROR'], settings.logLevel, (value) => {
            void saveSettings({ ...settings, logLevel: value });
          }),
        ),
      ),
      el(
        'div',
        { class: 'settings-list settings-section' },
        checkbox('Pause while writing a note', settings.pauseWhileComposingNote, (checked) => {
          void saveSettings({ ...settings, pauseWhileComposingNote: checked });
        }),
        checkbox('Show notes on the seek bar', settings.showNoteMarks, (checked) => {
          void saveSettings({ ...settings, showNoteMarks: checked });
        }),
        el(
          'label',
          { class: 'settings-row' },
          el('span', {}, 'Stamp notes this many seconds earlier'),
          numberBox(settings.noteCaptureOffset, 0, 60, (value) => {
            void saveSettings({ ...settings, noteCaptureOffset: value });
          }),
        ),
        el(
          'p',
          { class: 'menu-note' },
          'A moment is usually recognised as worth noting just after it passes. 0 uses the exact position.',
        ),
      ),
      el(
        'div',
        { class: 'settings-list settings-section' },
        checkbox('Check for updates automatically', settings.checkForUpdates, (checked) => {
          setCheckForUpdates(checked);
        }),
        versionRow(state),
      ),
      el(
        'p',
        { class: 'menu-note' },
        `Settings are stored in ${state.diagnostics?.settingsPath || 'your user application data folder'}.`,
      ),
    );
  }

  // --- Recent files ---

  private recentMenu(state: AppState): HTMLElement {
    const list = el('div', { class: 'recent-list' });

    if (state.recent.length === 0) {
      list.append(el('p', { class: 'menu-empty' }, 'Nothing has been opened yet.'));
    }

    for (const entry of state.recent) {
      const meta: string[] = [];
      if (entry.position > 0) meta.push(`stopped at ${formatTime(entry.position)}`);
      if (!entry.exists) meta.push('file not found');

      const row = el(
        'div',
        { class: entry.exists ? 'recent-row' : 'recent-row missing' },
        el(
          'button',
          {
            class: 'recent-open',
            type: 'button',
            title: entry.path,
            disabled: !entry.exists,
          },
          el('span', { class: 'recent-name' }, entry.title || entry.filename),
          el('span', { class: 'recent-meta' }, meta.join(' · ')),
        ),
        el('button', { class: 'recent-forget', type: 'button', title: 'Remove from this list' }, icon('close')),
      );

      row.querySelector('.recent-open')?.addEventListener('click', () => {
        void openPath(entry.path);
        close();
      });
      row.querySelector('.recent-forget')?.addEventListener('click', (event) => {
        event.stopPropagation();
        void forgetRecent(entry.path);
      });

      list.append(row);
    }

    return el(
      'div',
      { class: 'menu' },
      menuHeader('Recent files'),
      list,
      state.recent.length > 0
        ? el(
            'div',
            { class: 'menu-footer' },
            actionButton('close', 'Clear the list', () => {
              void clearRecent();
            }),
          )
        : null,
    );
  }

  // --- Resume prompt ---

  private resumePrompt(state: AppState): HTMLElement {
    const prompt = state.resumePrompt;
    if (!prompt) return el('div');

    const resume = el('button', { class: 'primary-button', type: 'button' },
      `Resume from ${formatTime(prompt.position)}`);
    resume.addEventListener('click', () => void acceptResume(prompt.position));

    const restart = el('button', { class: 'secondary-button', type: 'button' }, 'Start from beginning');
    restart.addEventListener('click', () => void declineResume());

    return el(
      'div',
      { class: 'menu resume-menu' },
      el(
        'div',
        { class: 'resume-text' },
        el('p', { class: 'resume-title' }, `You were watching ${prompt.filename}`),
        el('p', { class: 'resume-detail' }, `Last stopped at ${formatTime(prompt.position)}.`),
      ),
      el('div', { class: 'resume-actions' }, resume, restart),
      checkbox('Always resume automatically', state.settings.autoResume, (checked) => {
        setAutoResume(checked);
      }),
    );
  }
}

/**
 * Shows the running version and offers a check.
 *
 * The result is written straight into the row rather than only into the update
 * bar, because "you are up to date" is an answer to a question the user just
 * asked, and a bar that stays hidden looks like nothing happened.
 */
function versionRow(state: AppState): HTMLElement {
  const status = el('span', { class: 'settings-version-status' },
    state.update?.status.message ?? '');

  const check = el('button', { class: 'menu-action', type: 'button' }, 'Check now');
  check.addEventListener('click', () => {
    check.disabled = true;
    status.textContent = 'Checking…';

    void checkForUpdates(true).then((info) => {
      check.disabled = false;
      status.textContent = info?.status.message ?? 'The check could not be completed.';
    });
  });

  return el(
    'div',
    { class: 'settings-row settings-version' },
    el('span', { class: 'settings-version-label' },
      `Version ${state.update?.detail || state.update?.version || 'unknown'}`),
    check,
    status,
  );
}

function close(): void {
  store.set({ drawer: 'none' });
}

function menuHeader(title: string): HTMLElement {
  const closeButton = el('button', { class: 'menu-close', type: 'button', title: 'Close' }, icon('close'));
  closeButton.addEventListener('click', close);
  return el('div', { class: 'menu-header' }, el('h3', {}, title), closeButton);
}

function option(label: string, selected: boolean, onClick: () => void): HTMLElement {
  const node = el(
    'button',
    {
      class: selected ? 'menu-option selected' : 'menu-option',
      type: 'button',
      role: 'menuitemradio',
      'aria-checked': selected ? 'true' : 'false',
    },
    el('span', { class: 'menu-check' }, selected ? icon('check') : null),
    el('span', { class: 'menu-option-label' }, label),
  );
  node.addEventListener('click', onClick);
  return node;
}

function actionButton(iconName: string, label: string, onClick: () => void): HTMLElement {
  const node = el('button', { class: 'menu-action', type: 'button' }, icon(iconName), label);
  node.addEventListener('click', onClick);
  return node;
}

function checkbox(label: string, checked: boolean, onChange: (checked: boolean) => void): HTMLElement {
  const input = el('input', { type: 'checkbox' }) as HTMLInputElement;
  input.checked = checked;
  input.addEventListener('change', () => onChange(input.checked));
  return el('label', { class: 'settings-row checkbox-row' }, input, el('span', {}, label));
}

function selectBox(options: string[], value: string, onChange: (value: string) => void): HTMLElement {
  const select = el('select', { class: 'settings-select' }) as HTMLSelectElement;
  for (const option of options) {
    const node = el('option', { value: option }, option) as HTMLOptionElement;
    node.selected = option === value;
    select.append(node);
  }
  select.addEventListener('change', () => onChange(select.value));
  return select;
}

/**
 * A whole-number field, clamped to a range.
 *
 * Committed on change rather than on every keystroke, so half-typed values like
 * "1" on the way to "15" are not written to the settings file.
 */
function numberBox(value: number, min: number, max: number, onChange: (value: number) => void): HTMLElement {
  const input = el('input', {
    class: 'settings-number',
    type: 'number',
    min: String(min),
    max: String(max),
    step: '1',
  }) as HTMLInputElement;
  input.value = String(value);

  input.addEventListener('change', () => {
    const parsed = Number.parseInt(input.value, 10);
    const next = Number.isFinite(parsed) ? Math.min(Math.max(parsed, min), max) : value;
    input.value = String(next);
    onChange(next);
  });

  return input;
}

/** A -/+ stepper for a subtitle or audio offset, in tenths of a second. */
function delayControl(label: string, value: number, onChange: (value: number) => void): HTMLElement {
  const readout = el('span', { class: 'delay-value' }, formatDelay(value));

  const step = (delta: number) => {
    const next = Math.round((value + delta) * 10) / 10;
    readout.textContent = formatDelay(next);
    onChange(next);
  };

  const minus = el('button', { class: 'delay-button', type: 'button', title: 'Earlier' }, '−');
  minus.addEventListener('click', () => step(-0.1));

  const plus = el('button', { class: 'delay-button', type: 'button', title: 'Later' }, '+');
  plus.addEventListener('click', () => step(0.1));

  const reset = el('button', { class: 'delay-reset', type: 'button', title: 'Reset' }, 'Reset');
  reset.addEventListener('click', () => {
    readout.textContent = formatDelay(0);
    onChange(0);
  });

  return el('div', { class: 'delay-control' },
    el('span', { class: 'delay-label' }, label), minus, readout, plus, reset);
}

/**
 * A -/+ stepper for the subtitle size, shown as a percentage.
 *
 * A percentage reads more naturally than mpv's multiplier: "120%" says what it
 * does, where "1.2" invites the question "1.2 of what?".
 */
function scaleControl(label: string, value: number, onChange: (value: number) => void): HTMLElement {
  // A missing value means the backend has not reported yet; mpv's own size is 1.
  let current = value > 0 ? value : 1;

  const readout = el('span', { class: 'delay-value' }, formatScale(current));

  const step = (factor: number) => {
    const next = clampScale(Math.round(current * factor * 100) / 100);
    if (next === current) return;
    current = next;
    readout.textContent = formatScale(current);
    onChange(current);
  };

  // Multiplied rather than added: a fixed step is coarse at 25% and far too
  // fine at 400%, whereas a ratio feels even across the whole range.
  const smaller = el('button', { class: 'delay-button', type: 'button', title: 'Smaller' }, '−');
  smaller.addEventListener('click', () => step(1 / 1.1));

  const larger = el('button', { class: 'delay-button', type: 'button', title: 'Larger' }, '+');
  larger.addEventListener('click', () => step(1.1));

  const reset = el('button', { class: 'delay-reset', type: 'button', title: 'Back to the default size' }, 'Reset');
  reset.addEventListener('click', () => {
    current = 1;
    readout.textContent = formatScale(current);
    onChange(current);
  });

  return el('div', { class: 'delay-control' },
    el('span', { class: 'delay-label' }, label), smaller, readout, larger, reset);
}

function clampScale(value: number): number {
  return Math.min(Math.max(value, SUBTITLE_SCALE_MIN), SUBTITLE_SCALE_MAX);
}

function formatScale(value: number): string {
  return `${Math.round(value * 100)}%`;
}

function formatDelay(value: number): string {
  const rounded = Math.round(value * 10) / 10;
  if (rounded === 0) return '0.0s';
  return `${rounded > 0 ? '+' : ''}${rounded.toFixed(1)}s`;
}
