import { store } from '../state/store';
import type { AppState } from '../state/store';
import {
  dismissUpdate,
  installUpdate,
  openReleasePage,
  skipUpdate,
} from '../services/player';
import { clear, el } from '../util/dom';
import { formatBytes } from '../util/time';
import { icon } from './icons';

/**
 * Announces a newer release, and runs the update.
 *
 * It sits under the title bar and pushes the layout down rather than floating
 * over anything, for the same reason as the error banner: the video is a native
 * window that HTML cannot be drawn on top of.
 */
export class UpdateBar {
  readonly root: HTMLElement;
  private signature = '';

  constructor() {
    this.root = el('div', { class: 'update-bar', hidden: true });
  }

  mount(): void {
    store.subscribe((state) => this.render(state));
  }

  private render(state: AppState): void {
    const release = state.update?.status.available ? state.update.status.latest : null;
    const visible = Boolean(release) && !state.updateDismissed;

    this.root.hidden = !visible;
    if (!visible || !release) {
      this.signature = '';
      return;
    }

    // Rebuilt only when something it shows changes, so the buttons are not
    // replaced under the pointer on every state tick.
    const signature = [
      release.version,
      state.updateInstalling,
      state.updateProgress?.done ?? -1,
      state.update?.installed,
    ].join('|');
    if (signature === this.signature) return;
    this.signature = signature;

    clear(this.root);
    this.root.append(
      state.updateInstalling ? this.installingView(state) : this.offerView(state),
    );
  }

  /** What is shown before the user commits to anything. */
  private offerView(state: AppState): HTMLElement {
    const release = state.update!.status.latest!;
    const portable = state.update!.installed === false;

    const message = portable
      ? `PlayerOne ${release.version} is available. This is a portable copy, so update it from the release page.`
      : `PlayerOne ${release.version} is available. You have ${state.update!.version}.`;

    const actions = el('div', { class: 'update-actions' });

    const notes = textButton("What's new", () => void openReleasePage());
    actions.append(notes);

    if (!portable) {
      const install = el('button', { class: 'primary-button update-install', type: 'button' },
        `Update now (${formatBytes(release.installerSize)})`);
      install.addEventListener('click', () => void installUpdate());
      actions.append(install);
    } else {
      const open = el('button', { class: 'primary-button', type: 'button' }, 'Open the release page');
      open.addEventListener('click', () => void openReleasePage());
      actions.append(open);
    }

    actions.append(textButton('Skip this version', () => skipUpdate(release.version)));

    const dismiss = el('button', { class: 'banner-dismiss', type: 'button', title: 'Later' }, icon('close'));
    dismiss.addEventListener('click', () => dismissUpdate());

    return el(
      'div',
      { class: 'update-inner' },
      el('span', { class: 'update-icon' }, icon('download')),
      el('span', { class: 'update-text' }, message),
      actions,
      dismiss,
    );
  }

  /** What is shown while the installer is downloading. */
  private installingView(state: AppState): HTMLElement {
    const progress = state.updateProgress;
    const done = progress?.done ?? 0;
    const total = progress?.total ?? 0;

    const ratio = total > 0 ? Math.min(done / total, 1) : 0;
    const label = total > 0
      ? `Downloading ${formatBytes(done)} of ${formatBytes(total)}…`
      : 'Downloading the update…';

    const fill = el('div', { class: 'update-progress-fill' });
    fill.style.width = `${ratio * 100}%`;

    return el(
      'div',
      { class: 'update-inner' },
      el('span', { class: 'update-icon' }, icon('download')),
      el(
        'div',
        { class: 'update-progress-wrap' },
        el('span', { class: 'update-text' }, label),
        el('div', { class: 'update-progress' }, fill),
      ),
      el(
        'span',
        { class: 'update-note' },
        'PlayerOne will close so the installer can replace it, then reopen.',
      ),
    );
  }
}

function textButton(label: string, onClick: () => void): HTMLElement {
  const node = el('button', { class: 'update-link', type: 'button' }, label);
  node.addEventListener('click', onClick);
  return node;
}
