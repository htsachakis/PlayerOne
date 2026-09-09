import { store } from '../state/store';
import type { AppState } from '../state/store';
import { seekChapter } from '../services/player';
import { clear, el, highlight } from '../util/dom';
import { formatTimePadded } from '../util/time';

/**
 * The chapter list.
 *
 * Rows are built once per chapter list and then only have their active class
 * updated, because a two-hour tutorial can have sixty chapters and this reacts
 * to state five times a second.
 */
export class ChaptersTab {
  readonly root: HTMLElement;

  private readonly list: HTMLElement;
  private readonly empty: HTMLElement;
  private readonly search: HTMLInputElement;

  private rows: HTMLElement[] = [];
  private signature = '';
  private activeIndex = -1;
  private userScrolled = false;

  constructor() {
    this.search = el('input', {
      class: 'panel-search-input',
      type: 'search',
      placeholder: 'Search chapters',
      'aria-label': 'Search chapters',
    }) as HTMLInputElement;

    this.search.addEventListener('input', () => {
      store.set({ chapterQuery: this.search.value });
    });

    this.list = el('div', { class: 'chapter-list' });
    this.empty = el('div', { class: 'panel-empty' });

    this.root = el(
      'div',
      { class: 'panel-tab-content' },
      el('div', { class: 'panel-search' }, this.search),
      this.list,
      this.empty,
    );

    // A manual scroll suspends auto-following until the next chapter change, so
    // browsing the list is not fought by playback.
    this.list.addEventListener('scroll', () => {
      this.userScrolled = true;
    });
  }

  mount(): void {
    store.subscribe((state) => this.render(state));
  }

  private render(state: AppState): void {
    const query = state.chapterQuery.trim().toLowerCase();
    const signature = `${state.chapters.length}|${state.chapters[0]?.title ?? ''}|${query}|${state.playback.path}`;

    if (signature !== this.signature) {
      this.signature = signature;
      this.build(state, query);
      this.activeIndex = -1;
      this.userScrolled = false;
    }

    this.updateActive(state);
  }

  private build(state: AppState, query: string): void {
    clear(this.list);
    this.rows = [];

    if (state.chapters.length === 0) {
      this.empty.hidden = false;
      clear(this.empty);
      this.empty.append(
        el('p', { class: 'panel-empty-title' }, 'No chapters found in this video.'),
        el(
          'p',
          { class: 'panel-empty-detail' },
          state.playback.fileLoaded
            ? 'This file has no embedded chapter markers. The transcript tab may still let you jump around.'
            : 'Open a video to see its chapters.',
        ),
      );
      this.root.querySelector('.panel-search')?.setAttribute('hidden', '');
      return;
    }

    this.empty.hidden = true;
    this.root.querySelector('.panel-search')?.removeAttribute('hidden');

    const matches = query
      ? state.chapters.filter((c) => c.title.toLowerCase().includes(query))
      : state.chapters;

    if (matches.length === 0) {
      this.empty.hidden = false;
      clear(this.empty);
      this.empty.append(el('p', { class: 'panel-empty-title' }, `No chapters match “${query}”.`));
      return;
    }

    for (const chapter of matches) {
      const row = el(
        'button',
        {
          class: 'chapter-row',
          type: 'button',
          'data-index': String(chapter.index),
          title: chapter.title,
        },
        el('span', { class: 'chapter-time' }, formatTimePadded(chapter.start)),
        el('span', { class: 'chapter-title' }, highlight(chapter.title, query)),
      );

      row.addEventListener('click', () => {
        void seekChapter(chapter.index);
      });

      this.rows.push(row);
      this.list.append(row);
    }
  }

  private updateActive(state: AppState): void {
    const active = state.playback.chapterIndex;
    if (active === this.activeIndex) return;
    this.activeIndex = active;

    let activeRow: HTMLElement | null = null;
    for (const row of this.rows) {
      const isActive = Number(row.dataset.index) === active;
      row.classList.toggle('active', isActive);
      if (isActive) activeRow = row;
    }

    // Following the active chapter is always wanted here: unlike the
    // transcript, chapters change rarely, so a scroll every few minutes is
    // helpful rather than intrusive.
    if (activeRow && !this.userScrolled) {
      activeRow.scrollIntoView({ block: 'nearest' });
    } else if (activeRow) {
      // A chapter boundary is a natural moment to resume following.
      this.userScrolled = false;
      activeRow.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }
  }
}
