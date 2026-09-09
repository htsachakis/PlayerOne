import { store } from '../state/store';
import type { AppState } from '../state/store';
import { setVideoBounds, setVideoVisible } from '../services/player';
import { el, clear } from '../util/dom';

/**
 * The slot the native mpv window occupies, and the states shown in its place.
 *
 * mpv renders into a native child window that sits above the WebView, so
 * nothing here can be drawn on top of playing video. This component therefore
 * does two things: it keeps the backend informed of exactly where the video
 * rectangle is, and it hides the video whenever something else needs to be
 * visible in that space.
 */
export class VideoSurface {
  readonly root: HTMLElement;
  private readonly slot: HTMLElement;
  private readonly placeholder: HTMLElement;

  private observer: ResizeObserver | null = null;
  private lastRect = '';
  private videoShown = false;
  private engineWasReady = false;

  constructor() {
    this.slot = el('div', { class: 'video-slot', id: 'video-slot' });
    this.placeholder = el('div', { class: 'video-placeholder' });

    this.slot.append(this.placeholder);
    this.root = el('div', { class: 'video-area' }, this.slot);
  }

  mount(): void {
    this.observer = new ResizeObserver(() => this.publishBounds());
    this.observer.observe(this.slot);

    // A resize alone does not cover a window move, and the rectangle the
    // backend needs is viewport-relative.
    window.addEventListener('resize', this.publishBounds);
    window.addEventListener('scroll', this.publishBounds, true);

    store.subscribe((state) => this.render(state));
    this.publishBounds();
  }

  destroy(): void {
    this.observer?.disconnect();
    window.removeEventListener('resize', this.publishBounds);
    window.removeEventListener('scroll', this.publishBounds, true);
  }

  /** Reports the slot's rectangle to the backend, which moves the mpv window. */
  private publishBounds = (): void => {
    const rect = this.slot.getBoundingClientRect();

    // Layout settles over several frames during a panel drag or a fullscreen
    // change; skipping identical rectangles keeps that from becoming a stream
    // of redundant SetWindowPos calls.
    const key = `${rect.left}|${rect.top}|${rect.width}|${rect.height}|${window.devicePixelRatio}`;
    if (key === this.lastRect) return;
    this.lastRect = key;

    setVideoBounds(rect);
  };

  private render(state: AppState): void {
    // The layout is measured as soon as the page renders, which is before the
    // engine has finished starting. The backend keeps that first measurement
    // and applies it when the window appears; re-sending it here as well means
    // the video is placed correctly even if that ordering ever changes.
    if (state.engineReady && !this.engineWasReady) {
      this.engineWasReady = true;
      this.lastRect = '';
      requestAnimationFrame(this.publishBounds);
    }

    // The video must be hidden for anything else to be visible in this space:
    // a native window cannot be covered by HTML.
    const shouldShow = state.engineReady && state.playback.fileLoaded && !state.opening;

    if (shouldShow !== this.videoShown) {
      this.videoShown = shouldShow;
      setVideoVisible(shouldShow);
    }

    this.placeholder.hidden = shouldShow;
    if (shouldShow) return;

    this.renderPlaceholder(state);

    // The layout may have changed while the placeholder was swapped in.
    requestAnimationFrame(this.publishBounds);
  }

  private renderPlaceholder(state: AppState): void {
    clear(this.placeholder);

    if (!state.engineChecked) {
      this.placeholder.append(
        el('div', { class: 'empty-state' },
          el('div', { class: 'spinner' }),
          el('p', { class: 'empty-title' }, 'Starting the media engine…'),
        ),
      );
      return;
    }

    if (!state.engineReady) {
      this.placeholder.append(
        el('div', { class: 'empty-state error-state' },
          el('div', { class: 'empty-icon' }, '⚠'),
          el('p', { class: 'empty-title' }, 'The media engine is not available'),
          el('pre', { class: 'empty-detail' }, state.startupError || 'mpv could not be started.'),
        ),
      );
      return;
    }

    if (state.opening) {
      this.placeholder.append(
        el('div', { class: 'empty-state' },
          el('div', { class: 'spinner' }),
          el('p', { class: 'empty-title' }, 'Opening…'),
        ),
      );
      return;
    }

    this.placeholder.append(
      el('div', { class: 'empty-state' },
        el('div', { class: 'empty-logo' }, logoMark()),
        el('p', { class: 'empty-title' }, 'Open a video to begin'),
        el('p', { class: 'empty-detail' },
          'Use File ▸ Open, press Ctrl+O, or drop a video file onto this window.'),
        el('p', { class: 'empty-hint' },
          'MKV, MP4, WebM and anything else mpv can play. Chapters, subtitle tracks and transcripts are read automatically.'),
      ),
    );
  }
}

/**
 * The application mark: a play triangle beside a list, which is the whole idea
 * of the player in one shape.
 */
function logoMark(): SVGElement {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', '0 0 64 64');
  svg.setAttribute('width', '72');
  svg.setAttribute('height', '72');
  svg.setAttribute('aria-hidden', 'true');
  svg.innerHTML = `
    <defs>
      <linearGradient id="p1-empty-grad" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0%" stop-color="#5eb8ff"/>
        <stop offset="100%" stop-color="#2f7bff"/>
      </linearGradient>
    </defs>
    <rect x="2" y="6" width="60" height="52" rx="12" fill="none"
          stroke="currentColor" stroke-opacity="0.35" stroke-width="2"/>
    <path d="M18 20 L38 32 L18 44 Z" fill="url(#p1-empty-grad)"/>
    <g fill="currentColor" fill-opacity="0.35">
      <rect x="44" y="21" width="12" height="3" rx="1.5"/>
      <rect x="44" y="30.5" width="12" height="3" rx="1.5"/>
      <rect x="44" y="40" width="12" height="3" rx="1.5"/>
    </g>`;
  return svg;
}
