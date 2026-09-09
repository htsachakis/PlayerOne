import { store } from '../state/store';
import type { AppState } from '../state/store';
import {
  addFilesToPlaylist,
  addFolderToPlaylist,
  clearPlaylist,
  cycleRepeat,
  playPlaylistItem,
  removeFromPlaylist,
  setRepeat,
  setShuffle,
} from '../services/player';
import { clear, el } from '../util/dom';
import { formatTime } from '../util/time';
import { icon } from './icons';
import type { RepeatMode } from '../types/media';

/**
 * The queue of files to play, with shuffle and repeat.
 *
 * It lives beside Chapters and Transcript because all three answer "what can I
 * jump to from here?", just at different scales: within the file, within the
 * spoken words, and across the course.
 */
export class PlaylistTab {
  readonly root: HTMLElement;

  private readonly list: HTMLElement;
  private readonly empty: HTMLElement;
  private readonly shuffleButton: HTMLButtonElement;
  private readonly repeatButton: HTMLButtonElement;
  private readonly countLabel: HTMLElement;

  private signature = '';

  constructor() {
    this.shuffleButton = toolButton('shuffle', 'Shuffle', () => {
      void setShuffle(!store.get().playlist.shuffle);
    });

    this.repeatButton = toolButton('repeat', 'Repeat', () => {
      void setRepeat(cycleRepeat(store.get().playlist.repeat));
    });

    const addFiles = toolButton('open', 'Add files…', () => void addFilesToPlaylist());
    const addFolder = toolButton('folder', 'Add a folder…', () => void addFolderToPlaylist());
    const clearAll = toolButton('close', 'Clear the playlist', () => void clearPlaylist());

    this.countLabel = el('span', { class: 'playlist-count' });

    this.list = el('div', { class: 'playlist-list' });
    this.empty = el('div', { class: 'panel-empty' });

    this.root = el(
      'div',
      { class: 'panel-tab-content' },
      el(
        'div',
        { class: 'playlist-toolbar' },
        this.shuffleButton,
        this.repeatButton,
        el('span', { class: 'playlist-spacer' }),
        this.countLabel,
        addFiles,
        addFolder,
        clearAll,
      ),
      this.list,
      this.empty,
    );
  }

  mount(): void {
    store.subscribe((state) => this.render(state));
  }

  private render(state: AppState): void {
    const { playlist } = state;

    this.shuffleButton.classList.toggle('active', playlist.shuffle);
    this.shuffleButton.title = playlist.shuffle
      ? 'Shuffle is on — click to play in order'
      : 'Shuffle the playlist';
    this.shuffleButton.setAttribute('aria-pressed', playlist.shuffle ? 'true' : 'false');

    this.repeatButton.classList.toggle('active', playlist.repeat !== 'off');
    setRepeatIcon(this.repeatButton, playlist.repeat);
    this.repeatButton.title = repeatTitle(playlist.repeat);

    const signature = [
      playlist.items.map((i) => `${i.path}|${i.missing}|${i.duration}|${i.title}`).join('~'),
      playlist.current,
    ].join('#');

    if (signature !== this.signature) {
      this.signature = signature;
      this.build(state);
    }

    this.countLabel.textContent =
      playlist.items.length === 0
        ? ''
        : `${playlist.items.length} ${playlist.items.length === 1 ? 'item' : 'items'}`;
  }

  private build(state: AppState): void {
    const { playlist } = state;

    clear(this.list);
    clear(this.empty);

    if (playlist.items.length === 0) {
      this.empty.hidden = false;
      this.empty.append(
        el('p', { class: 'panel-empty-title' }, 'The playlist is empty'),
        el(
          'p',
          { class: 'panel-empty-detail' },
          'Add files or a folder with the buttons above, or drop several videos onto the window. ' +
            'A folder queues its videos in natural order, so lesson 2 comes before lesson 10.',
        ),
      );
      return;
    }

    this.empty.hidden = true;

    const fragment = document.createDocumentFragment();

    playlist.items.forEach((item, index) => {
      const isCurrent = index === playlist.current;

      const meta: string[] = [];
      if (item.duration > 0) meta.push(formatTime(item.duration));
      if (item.missing) meta.push('file not found');

      const row = el(
        'div',
        {
          class: [
            'playlist-row',
            isCurrent ? 'current' : '',
            item.missing ? 'missing' : '',
          ]
            .filter(Boolean)
            .join(' '),
        },
        el('span', { class: 'playlist-index' }, isCurrent ? icon('play') : String(index + 1)),
        el(
          'button',
          { class: 'playlist-open', type: 'button', title: item.path, disabled: item.missing },
          el('span', { class: 'playlist-name' }, item.title || item.filename),
          el('span', { class: 'playlist-meta' }, meta.join(' · ')),
        ),
        el('button', { class: 'playlist-remove', type: 'button', title: 'Remove from the playlist' }, icon('close')),
      );

      row.querySelector('.playlist-open')?.addEventListener('click', () => {
        void playPlaylistItem(index);
      });
      row.querySelector('.playlist-remove')?.addEventListener('click', (event) => {
        event.stopPropagation();
        void removeFromPlaylist(index);
      });

      fragment.append(row);
    });

    this.list.append(fragment);

    // Keep the playing entry in view when the queue changes underneath it.
    requestAnimationFrame(() => {
      this.list.querySelector('.playlist-row.current')?.scrollIntoView({ block: 'nearest' });
    });
  }
}

function toolButton(name: string, title: string, onClick: () => void): HTMLButtonElement {
  const node = el('button', { class: 'playlist-tool', type: 'button', title, 'aria-label': title });
  node.append(icon(name));
  node.addEventListener('click', onClick);
  return node as HTMLButtonElement;
}

function setRepeatIcon(node: HTMLElement, mode: RepeatMode): void {
  const name = mode === 'one' ? 'repeatOne' : 'repeat';
  const existing = node.querySelector('svg');
  if (existing?.dataset.icon === name) return;

  const next = icon(name);
  if (existing) existing.replaceWith(next);
  else node.prepend(next);
}

function repeatTitle(mode: RepeatMode): string {
  switch (mode) {
    case 'all':
      return 'Repeat the whole playlist — click to repeat one';
    case 'one':
      return 'Repeat this file — click to turn repeat off';
    default:
      return 'Repeat is off — click to repeat the playlist';
  }
}
