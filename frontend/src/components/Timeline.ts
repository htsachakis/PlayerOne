import { store } from '../state/store';
import type { AppState } from '../state/store';
import { seek } from '../services/player';
import { clear, el, setText } from '../util/dom';
import { formatTime } from '../util/time';

/**
 * The seek bar, with chapter marks and a scrub preview.
 *
 * While the user is dragging, the bar shows the dragged position rather than
 * the backend's, so the handle tracks the pointer instead of snapping back and
 * forth as five-per-second state updates arrive. The backend remains
 * authoritative the moment the drag ends.
 */
export class Timeline {
  readonly root: HTMLElement;

  private readonly track: HTMLElement;
  private readonly played: HTMLElement;
  private readonly handle: HTMLElement;
  private readonly marks: HTMLElement;
  private readonly tooltip: HTMLElement;
  private readonly current: HTMLElement;
  private readonly total: HTMLElement;

  private dragging = false;
  private dragPosition = 0;
  private duration = 0;
  private chapterSignature = '';

  constructor() {
    this.played = el('div', { class: 'timeline-played' });
    this.handle = el('div', { class: 'timeline-handle' });
    this.marks = el('div', { class: 'timeline-marks' });
    this.tooltip = el('div', { class: 'timeline-tooltip', hidden: true });

    this.track = el(
      'div',
      {
        class: 'timeline-track',
        role: 'slider',
        tabindex: '-1',
        'aria-label': 'Playback position',
      },
      el('div', { class: 'timeline-rail' }),
      this.played,
      this.marks,
      this.handle,
      this.tooltip,
    );

    this.current = el('span', { class: 'time-current' }, '0:00');
    this.total = el('span', { class: 'time-total' }, '0:00');

    this.root = el('div', { class: 'timeline' }, this.current, this.track, this.total);
  }

  mount(): void {
    this.track.addEventListener('pointerdown', this.onPointerDown);
    this.track.addEventListener('pointermove', this.onHover);
    this.track.addEventListener('pointerleave', () => {
      this.tooltip.hidden = true;
    });

    store.subscribe((state) => this.render(state));
  }

  private positionFromEvent(event: PointerEvent | MouseEvent): number {
    const rect = this.track.getBoundingClientRect();
    if (rect.width <= 0 || this.duration <= 0) return 0;

    const ratio = clamp((event.clientX - rect.left) / rect.width, 0, 1);
    return ratio * this.duration;
  }

  private onPointerDown = (event: PointerEvent): void => {
    if (this.duration <= 0) return;

    this.dragging = true;
    this.dragPosition = this.positionFromEvent(event);
    this.track.setPointerCapture(event.pointerId);
    this.paint(this.dragPosition, this.duration);

    this.track.addEventListener('pointermove', this.onDragMove);
    this.track.addEventListener('pointerup', this.onDragEnd);
    this.track.addEventListener('pointercancel', this.onDragEnd);
    event.preventDefault();
  };

  private onDragMove = (event: PointerEvent): void => {
    if (!this.dragging) return;
    this.dragPosition = this.positionFromEvent(event);
    this.paint(this.dragPosition, this.duration);
    this.showTooltip(event, this.dragPosition);
  };

  private onDragEnd = (event: PointerEvent): void => {
    if (!this.dragging) return;

    this.dragging = false;
    this.track.releasePointerCapture?.(event.pointerId);
    this.track.removeEventListener('pointermove', this.onDragMove);
    this.track.removeEventListener('pointerup', this.onDragEnd);
    this.track.removeEventListener('pointercancel', this.onDragEnd);
    this.tooltip.hidden = true;

    void seek(this.dragPosition);
  };

  private onHover = (event: PointerEvent): void => {
    if (this.dragging || this.duration <= 0) return;
    this.showTooltip(event, this.positionFromEvent(event));
  };

  /**
   * Positions the tooltip over the cursor, without letting it leave the window.
   *
   * The tooltip is centred on the pointer, so near either end of the bar half
   * of it would hang past the window edge and be clipped - exactly where the
   * first and last chapters are. Clamping happens in viewport coordinates
   * because that is where the edges are; the result is converted back to the
   * track's own coordinates at the end.
   */
  private showTooltip(event: PointerEvent | MouseEvent, position: number): void {
    // The text is set first: the clamp needs the tooltip's real width, and
    // that is only known once it has content and is laid out.
    setText(this.tooltip, this.tooltipLabel(position));
    this.tooltip.hidden = false;

    const rect = this.track.getBoundingClientRect();
    const half = this.tooltip.offsetWidth / 2;
    const centre = clamp(
      event.clientX,
      TOOLTIP_EDGE_MARGIN + half,
      window.innerWidth - TOOLTIP_EDGE_MARGIN - half,
    );

    this.tooltip.style.left = `${centre - rect.left}px`;
  }

  private tooltipLabel(position: number): string {
    const chapter = chapterAt(store.get().chapters, position);
    const time = formatTime(position);
    return chapter ? `${time} · ${chapter.title}` : time;
  }

  private render(state: AppState): void {
    const { position, duration } = state.playback;
    this.duration = duration;

    this.renderChapterMarks(state);

    if (this.dragging) return; // the pointer owns the handle until it is released
    this.paint(position, duration);
  }

  private paint(position: number, duration: number): void {
    const ratio = duration > 0 ? clamp(position / duration, 0, 1) : 0;
    const percent = `${ratio * 100}%`;

    this.played.style.width = percent;
    this.handle.style.left = percent;

    setText(this.current, formatTime(position));
    setText(this.total, formatTime(duration));

    this.track.setAttribute('aria-valuenow', String(Math.round(position)));
    this.track.setAttribute('aria-valuemax', String(Math.round(duration)));
    this.track.setAttribute('aria-valuetext', formatTime(position));
  }

  /**
   * Draws a tick for each chapter start.
   *
   * Rebuilt only when the chapters or duration actually change; with 59
   * chapters this would otherwise rebuild five times a second.
   */
  private renderChapterMarks(state: AppState): void {
    const duration = state.playback.duration;
    const signature = `${duration}|${state.chapters.length}|${state.chapters[0]?.title ?? ''}`;
    if (signature === this.chapterSignature) return;
    this.chapterSignature = signature;

    clear(this.marks);
    if (duration <= 0) return;

    for (const chapter of state.chapters) {
      if (chapter.start <= 0 || chapter.start >= duration) continue;
      const mark = el('span', { class: 'timeline-mark', title: chapter.title });
      mark.style.left = `${(chapter.start / duration) * 100}%`;
      this.marks.append(mark);
    }
  }
}

/** How close the tooltip may come to the window edge. */
const TOOLTIP_EDGE_MARGIN = 8;

function chapterAt(chapters: Array<{ start: number; title: string }>, position: number) {
  let found: { start: number; title: string } | null = null;
  for (const chapter of chapters) {
    if (chapter.start > position) break;
    found = chapter;
  }
  return found;
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
