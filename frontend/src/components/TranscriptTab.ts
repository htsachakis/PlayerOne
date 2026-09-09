import { store } from '../state/store';
import type { AppState } from '../state/store';
import { seek, setFollowTranscript } from '../services/player';
import { clear, el, findActiveIndex, highlight } from '../util/dom';
import { formatTimePadded } from '../util/time';
import { icon } from './icons';

/**
 * How long a manual scroll suspends auto-following.
 *
 * Long enough to read a paragraph without the view being yanked away, short
 * enough that following resumes without the user having to do anything.
 */
const SCROLL_GRACE_MS = 4000;

/**
 * The transcript: every subtitle line, timestamped and clickable.
 *
 * Rows are created once and then only reclassified, because a two-hour tutorial
 * produces well over a thousand of them and the active line is recalculated
 * five times a second.
 */
export class TranscriptTab {
  readonly root: HTMLElement;

  private readonly list: HTMLElement;
  private readonly status: HTMLElement;
  private readonly search: HTMLInputElement;
  private readonly followButton: HTMLButtonElement;

  private rows: HTMLElement[] = [];
  private entries: Array<{ start: number; end: number }> = [];
  private signature = '';
  private activeIndex = -1;
  private lastManualScroll = 0;
  private programmaticScroll = false;

  constructor() {
    this.search = el('input', {
      class: 'panel-search-input',
      type: 'search',
      placeholder: 'Search transcript',
      'aria-label': 'Search transcript',
    }) as HTMLInputElement;

    this.search.addEventListener('input', () => {
      store.set({ transcriptQuery: this.search.value });
    });

    this.followButton = el('button', {
      class: 'follow-button',
      type: 'button',
      title: 'Keep the transcript in step with playback',
    }) as HTMLButtonElement;
    this.followButton.append(icon('check'), el('span', {}, 'Follow playback'));
    this.followButton.addEventListener('click', () => {
      const next = !store.get().followTranscript;
      setFollowTranscript(next);
      // Turning following back on should jump straight to the current line.
      if (next) this.lastManualScroll = 0;
    });

    this.list = el('div', { class: 'transcript-list' });
    this.status = el('div', { class: 'panel-empty' });

    this.list.addEventListener('scroll', () => {
      // Scrolls this component caused itself must not count as the user taking
      // over, or following would disable itself the moment it worked.
      if (this.programmaticScroll) return;
      this.lastManualScroll = Date.now();
    });

    this.root = el(
      'div',
      { class: 'panel-tab-content' },
      el('div', { class: 'panel-search' }, this.search, this.followButton),
      this.list,
      this.status,
    );
  }

  mount(): void {
    store.subscribe((state) => this.render(state));
  }

  private render(state: AppState): void {
    this.followButton.classList.toggle('active', state.followTranscript);
    this.followButton.setAttribute('aria-pressed', state.followTranscript ? 'true' : 'false');

    const query = state.transcriptQuery.trim().toLowerCase();
    const transcript = state.transcript;

    const signature = [
      state.transcriptLoading ? 'loading' : 'ready',
      transcript?.trackId ?? -1,
      transcript?.entries.length ?? 0,
      transcript?.status ?? '',
      query,
    ].join('|');

    if (signature !== this.signature) {
      this.signature = signature;
      this.build(state, query);
      this.activeIndex = -1;
    }

    this.updateActive(state);
  }

  private build(state: AppState, query: string): void {
    clear(this.list);
    clear(this.status);
    this.rows = [];
    this.entries = [];

    const searchRow = this.root.querySelector('.panel-search');

    if (state.transcriptLoading) {
      searchRow?.setAttribute('hidden', '');
      this.status.hidden = false;
      this.status.append(
        el('div', { class: 'spinner' }),
        el('p', { class: 'panel-empty-title' }, 'Reading the subtitle track…'),
        el('p', { class: 'panel-empty-detail' },
          'Subtitles embedded in the video are extracted with ffmpeg. This takes a few seconds for a long file.'),
      );
      return;
    }

    const transcript = state.transcript;

    if (!transcript || transcript.entries.length === 0) {
      searchRow?.setAttribute('hidden', '');
      this.status.hidden = false;
      this.status.append(
        el('p', { class: 'panel-empty-title' }, 'No transcript available'),
        el('p', { class: 'panel-empty-detail' },
          transcript?.status || 'Open a video with a text subtitle track to see its transcript.'),
      );
      return;
    }

    searchRow?.removeAttribute('hidden');
    this.status.hidden = true;

    const matches = query
      ? transcript.entries.filter((e) => e.text.toLowerCase().includes(query))
      : transcript.entries;

    if (matches.length === 0) {
      this.status.hidden = false;
      this.status.append(el('p', { class: 'panel-empty-title' }, `Nothing in the transcript matches “${query}”.`));
      return;
    }

    // A fragment keeps a thousand-plus rows to a single insertion.
    const fragment = document.createDocumentFragment();

    for (const entry of matches) {
      const row = el(
        'button',
        { class: 'transcript-row', type: 'button' },
        el('span', { class: 'transcript-time' }, formatTimePadded(entry.start)),
        el('span', { class: 'transcript-text' }, highlight(entry.text, query)),
      );

      row.addEventListener('click', () => {
        void seek(entry.start);
      });

      this.rows.push(row);
      this.entries.push({ start: entry.start, end: entry.end });
      fragment.append(row);
    }

    this.list.append(fragment);
  }

  private updateActive(state: AppState): void {
    if (this.entries.length === 0) return;

    const index = findActiveIndex(this.entries, state.playback.position);
    if (index === this.activeIndex) return;

    if (this.activeIndex >= 0 && this.rows[this.activeIndex]) {
      this.rows[this.activeIndex].classList.remove('active');
    }
    this.activeIndex = index;
    if (index < 0) return;

    const row = this.rows[index];
    if (!row) return;
    row.classList.add('active');

    if (!state.followTranscript) return;
    if (Date.now() - this.lastManualScroll < SCROLL_GRACE_MS) return;

    // The scroll listener would otherwise read this as the user taking over.
    this.programmaticScroll = true;
    row.scrollIntoView({ block: 'center', behavior: 'smooth' });
    window.setTimeout(() => {
      this.programmaticScroll = false;
    }, 400);
  }
}
