import { store } from '../state/store';
import type { AppState } from '../state/store';
import {
  deleteNote,
  loadNotesFrom,
  openNoteComposer,
  saveNotesAs,
  seek,
  toggleNoteStar,
} from '../services/player';
import type { Note } from '../types/media';
import { clear, el, findActiveIndex, highlight } from '../util/dom';
import { formatTimePadded } from '../util/time';
import { icon } from './icons';

/**
 * The notes a viewer has taken against this video.
 *
 * Built on the same shape as the transcript: a timestamped list that follows
 * playback and seeks when clicked. A bookmark is a note with no text, so the two
 * are one row type, with one of them showing a placeholder in place of words.
 */
export class NotesTab {
  readonly root: HTMLElement;

  private readonly list: HTMLElement;
  private readonly status: HTMLElement;
  private readonly search: HTMLInputElement;
  private readonly starFilter: HTMLButtonElement;
  private readonly searchRow: HTMLElement;
  private readonly count: HTMLElement;
  private readonly notice: HTMLElement;

  private rows: HTMLElement[] = [];
  private spans: Array<{ start: number; end: number }> = [];
  private signature = '';
  private activeIndex = -1;

  constructor() {
    this.search = el('input', {
      class: 'panel-search-input',
      type: 'search',
      placeholder: 'Search notes',
      'aria-label': 'Search notes',
    }) as HTMLInputElement;

    this.search.addEventListener('input', () => {
      store.set({ notesQuery: this.search.value });
    });

    this.starFilter = el('button', {
      class: 'follow-button',
      type: 'button',
      title: 'Show only starred notes',
    }) as HTMLButtonElement;
    this.starFilter.append(icon('star'), el('span', {}, 'Starred'));
    this.starFilter.addEventListener('click', () => {
      store.set({ notesStarredOnly: !store.get().notesStarredOnly });
    });

    this.searchRow = el('div', { class: 'panel-search' }, this.search, this.starFilter);
    this.count = el('div', { class: 'notes-count' });
    this.notice = el('div', { class: 'notes-notice' });
    this.list = el('div', { class: 'transcript-list' });
    this.status = el('div', { class: 'panel-empty' });

    const addButton = this.action('Add a note  (T)', 'note', () => void openNoteComposer());
    const loadButton = this.action('Load notes…', 'openPlaylist', () => void loadNotesFrom());
    const saveButton = this.action('Save notes as…', 'save', () => void saveNotesAs());

    this.root = el(
      'div',
      { class: 'panel-tab-content' },
      this.searchRow,
      this.count,
      this.notice,
      this.list,
      this.status,
      el('div', { class: 'notes-actions' }, addButton, loadButton, saveButton),
    );
  }

  mount(): void {
    store.subscribe((state) => this.render(state));
  }

  private action(label: string, iconName: string, onClick: () => void): HTMLButtonElement {
    const button = el('button', { class: 'notes-action', type: 'button' },
      icon(iconName), el('span', {}, label)) as HTMLButtonElement;
    button.addEventListener('click', onClick);
    return button;
  }

  private render(state: AppState): void {
    this.starFilter.classList.toggle('active', state.notesStarredOnly);
    this.starFilter.setAttribute('aria-pressed', state.notesStarredOnly ? 'true' : 'false');

    const query = state.notesQuery.trim().toLowerCase();
    const entries = state.notes?.entries ?? [];

    // Times and stars are what the rows are built from; text length is enough to
    // catch an edit without stringifying every note five times a second.
    const signature = [
      entries.map((n) => `${n.time}:${n.starred ? 1 : 0}:${n.text.length}`).join(','),
      query,
      state.notesStarredOnly ? 'starred' : 'all',
      state.notes?.status ?? '',
    ].join('|');

    if (signature !== this.signature) {
      this.signature = signature;
      this.build(state, entries, query);
      this.activeIndex = -1;
    }

    this.updateActive(state);
  }

  private build(state: AppState, entries: Note[], query: string): void {
    clear(this.list);
    clear(this.status);
    clear(this.notice);
    this.rows = [];
    this.spans = [];

    // The notice stays put rather than re-prompting on every note. Being asked
    // again on each keystroke would be worse than a bar that waits to be dealt
    // with.
    const status = state.notes?.status ?? '';
    this.notice.hidden = status === '';
    if (status !== '') {
      this.notice.append(
        el('p', {}, status),
        this.action('Save notes as…', 'save', () => void saveNotesAs()),
      );
    }

    if (entries.length === 0) {
      this.searchRow.hidden = true;
      this.count.hidden = true;
      this.status.hidden = false;
      this.status.append(
        el('p', { class: 'panel-empty-title' }, 'No notes yet'),
        el('p', { class: 'panel-empty-detail' },
          'Press T while watching to note the moment you are at. Save one with no text and it becomes a bookmark.'),
      );
      return;
    }

    this.searchRow.hidden = false;

    const starred = entries.filter((note) => note.starred).length;
    const matches = entries.filter((note) => {
      if (state.notesStarredOnly && !note.starred) return false;
      if (query !== '' && !note.text.toLowerCase().includes(query)) return false;
      return true;
    });

    // A bookmark has no text, so a search can never match it. The count is what
    // makes that legible rather than mysterious.
    this.count.hidden = false;
    this.count.textContent = matches.length === entries.length
      ? `${entries.length} ${entries.length === 1 ? 'note' : 'notes'} · ${starred} starred`
      : `${matches.length} of ${entries.length} notes`;

    this.status.hidden = matches.length > 0;
    if (matches.length === 0) {
      this.status.append(el('p', { class: 'panel-empty-title' }, 'No notes match that.'));
      return;
    }

    const fragment = document.createDocumentFragment();

    for (const note of matches) {
      fragment.append(this.buildRow(note, query));
    }

    // A note runs until the next one starts, which is what makes "the note you
    // are currently in" mean anything as playback moves.
    for (let i = 0; i < this.spans.length - 1; i += 1) {
      this.spans[i].end = this.spans[i + 1].start;
    }

    this.list.append(fragment);
  }

  private buildRow(note: Note, query: string): HTMLElement {
    const star = el('button', {
      class: note.starred ? 'note-star active' : 'note-star',
      type: 'button',
      title: note.starred ? 'Remove the star' : 'Star this note',
    }, icon(note.starred ? 'star' : 'starOutline')) as HTMLButtonElement;
    star.addEventListener('click', (event) => {
      event.stopPropagation();
      void toggleNoteStar(note.time);
    });

    const remove = el('button', {
      class: 'note-delete',
      type: 'button',
      title: 'Delete this note',
    }, icon('close')) as HTMLButtonElement;
    remove.addEventListener('click', (event) => {
      event.stopPropagation();
      void deleteNote(note.time);
    });

    const body = note.text === ''
      ? el('span', { class: 'transcript-text note-bookmark' }, 'Bookmark')
      : el('span', { class: 'transcript-text' }, highlight(note.text, query));

    const row = el(
      'div',
      { class: 'transcript-row note-row' },
      el('span', { class: 'transcript-time' }, formatTimePadded(note.time)),
      body,
      star,
      remove,
    );

    row.addEventListener('click', () => {
      void seek(note.time);
    });

    this.rows.push(row);
    this.spans.push({ start: note.time, end: Number.MAX_SAFE_INTEGER });
    return row;
  }

  private updateActive(state: AppState): void {
    if (this.spans.length === 0) return;

    const index = findActiveIndex(this.spans, state.playback.position);
    if (index === this.activeIndex) return;

    if (this.activeIndex >= 0 && this.rows[this.activeIndex]) {
      this.rows[this.activeIndex].classList.remove('active');
    }
    this.activeIndex = index;
    if (index >= 0) this.rows[index]?.classList.add('active');
  }
}
