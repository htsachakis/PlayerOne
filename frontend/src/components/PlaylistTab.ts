import { store } from '../state/store';
import type { AppState } from '../state/store';
import {
  addFilesToPlaylist,
  addFolderToPlaylist,
  clearPlaylist,
  exportPlaylist,
  importPlaylist,
  playPlaylistItem,
  removeFromPlaylist,
} from '../services/player';
import { clear, el } from '../util/dom';
import { formatTime } from '../util/time';
import { icon } from './icons';

/**
 * The queue of files to play.
 *
 * It lives beside Chapters and Transcript because all three answer "what can I
 * jump to from here?", just at different scales: within the file, within the
 * spoken words, and across the course. Shuffle and repeat are in the control
 * bar, where the rest of the playback controls are.
 */
export class PlaylistTab {
  readonly root: HTMLElement;

  private readonly list: HTMLElement;
  private readonly empty: HTMLElement;
  private readonly countLabel: HTMLElement;

  private signature = '';

  constructor() {
    // Shuffle and repeat live only in the control bar. Having them here as
    // well meant two sets of controls for one setting, and they crowded out
    // the actions below when the panel was narrow.
    const addFiles = toolButton('addFiles', 'Add files…', () => void addFilesToPlaylist());
    const addFolder = toolButton('folderPlus', 'Add a folder…', () => void addFolderToPlaylist());
    // Straight to the system dialogs: a playlist is a file, and the file
    // dialogs already do choosing a name and a location better than anything
    // built into the panel could.
    const save = toolButton('save', 'Save this playlist to a file…', () => void exportPlaylist());
    const open = toolButton('openPlaylist', 'Open a playlist file…', () => void importPlaylist());

    const clearAll = toolButton('trash', 'Clear the playlist', () => void clearPlaylist());

    this.countLabel = el('span', { class: 'playlist-count' });

    this.list = el('div', { class: 'playlist-list' });
    this.empty = el('div', { class: 'panel-empty' });

    this.root = el(
      'div',
      { class: 'panel-tab-content' },
      el(
        'div',
        { class: 'playlist-toolbar' },
        addFiles,
        addFolder,
        save,
        open,
        clearAll,
        this.countLabel,
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
