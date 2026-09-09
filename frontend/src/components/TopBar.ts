import { store } from '../state/store';
import type { AppState } from '../state/store';
import { chooseAndAddSubtitle, chooseAndOpen, clearError, refreshRecent, stop } from '../services/player';
import { el, setText } from '../util/dom';
import { icon } from './icons';

/**
 * The title strip: the application mark, what is playing, and the File actions.
 *
 * The error banner lives here too. It sits above the video and pushes the
 * layout down rather than overlapping, because the video is a native window
 * that HTML cannot be drawn on top of.
 */
export class TopBar {
  readonly root: HTMLElement;

  private readonly title: HTMLElement;
  private readonly subtitleFileButton: HTMLButtonElement;
  private readonly stopButton: HTMLButtonElement;
  private readonly banner: HTMLElement;
  private readonly bannerText: HTMLElement;

  constructor() {
    const openButton = action('open', 'Open', 'Open a video  (Ctrl+O)', () => void chooseAndOpen());

    const recentButton = action('folder', 'Recent', 'Recently opened files', () => {
      void refreshRecent();
      store.update((state) => ({ drawer: state.drawer === 'recent' ? 'none' : 'recent' }));
    });

    this.subtitleFileButton = action('subtitleFile', 'Subtitle', 'Load a subtitle file', () =>
      void chooseAndAddSubtitle(),
    );

    this.stopButton = action('close', 'Close', 'Close the current video', () => void stop());

    this.title = el('span', { class: 'now-playing-title' }, '');

    this.bannerText = el('span', { class: 'banner-text' });
    const dismiss = el('button', { class: 'banner-dismiss', type: 'button', title: 'Dismiss' }, icon('close'));
    dismiss.addEventListener('click', () => clearError());

    this.banner = el('div', { class: 'banner', hidden: true }, el('span', { class: 'banner-icon' }, '⚠'), this.bannerText, dismiss);

    this.root = el(
      'div',
      { class: 'topbar-wrap' },
      el(
        'header',
        { class: 'topbar' },
        el('div', { class: 'brand' }, brandMark(), el('span', { class: 'brand-name' }, 'PlayerOne')),
        el('div', { class: 'now-playing' }, this.title),
        el('div', { class: 'topbar-actions' }, openButton, recentButton, this.subtitleFileButton, this.stopButton),
      ),
      this.banner,
    );
  }

  mount(): void {
    store.subscribe((state) => this.render(state));
  }

  private render(state: AppState): void {
    const { playback } = state;

    setText(this.title, playback.fileLoaded ? playback.title || playback.path : '');
    this.title.title = playback.path;

    this.subtitleFileButton.disabled = !playback.fileLoaded;
    this.stopButton.disabled = !playback.fileLoaded;

    const hasError = Boolean(state.error);
    this.banner.hidden = !hasError;
    if (hasError) setText(this.bannerText, state.error ?? '');

    this.root.hidden = state.fullscreen;
  }
}

function action(iconName: string, label: string, title: string, onClick: () => void): HTMLButtonElement {
  const node = el('button', { class: 'topbar-button', type: 'button', title }, icon(iconName), el('span', {}, label));
  node.addEventListener('click', onClick);
  return node as HTMLButtonElement;
}

/**
 * The application mark: a play triangle beside a list, echoing the idea that
 * this is a player with a panel of contents.
 */
function brandMark(): SVGElement {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', '0 0 32 32');
  svg.setAttribute('width', '22');
  svg.setAttribute('height', '22');
  svg.setAttribute('aria-hidden', 'true');
  svg.classList.add('brand-mark');
  svg.innerHTML = `
    <defs>
      <linearGradient id="p1-brand-grad" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0%" stop-color="#6ec2ff"/>
        <stop offset="100%" stop-color="#2f7bff"/>
      </linearGradient>
    </defs>
    <rect x="1" y="3" width="30" height="26" rx="7" fill="#161b22"
          stroke="url(#p1-brand-grad)" stroke-width="1.4"/>
    <path d="M9 10.5 L18.5 16 L9 21.5 Z" fill="url(#p1-brand-grad)"/>
    <g fill="#9fb3c8" fill-opacity="0.75">
      <rect x="21.5" y="11" width="6.5" height="1.8" rx="0.9"/>
      <rect x="21.5" y="15.1" width="6.5" height="1.8" rx="0.9"/>
      <rect x="21.5" y="19.2" width="6.5" height="1.8" rx="0.9"/>
    </g>`;
  return svg;
}
