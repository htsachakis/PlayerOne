import { store } from '../state/store';
import type { AppState, PanelTab } from '../state/store';
import { persistSidePanel } from '../services/player';
import { el, toggleClass } from '../util/dom';
import { icon } from './icons';
import { ChaptersTab } from './ChaptersTab';
import { TranscriptTab } from './TranscriptTab';
import { NotesTab } from './NotesTab';
import { InfoTab } from './InfoTab';
import { PlaylistTab } from './PlaylistTab';

const MIN_WIDTH = 260;
const MAX_WIDTH = 900;

/**
 * The "In this video" panel: chapters, transcript, notes, the queue and file
 * details.
 *
 * Notes sits beside the transcript because the two are used the same way -
 * timestamped lists read while watching - and Info comes last, being reference
 * material consulted once rather than followed.
 *
 * The panel is resizable by dragging its inner edge, and hideable so the video
 * can use the full window width.
 */
export class SidePanel {
  readonly root: HTMLElement;
  readonly handle: HTMLElement;

  private readonly tabButtons = new Map<PanelTab, HTMLButtonElement>();
  private readonly titleNode: HTMLElement;
  private readonly chapters = new ChaptersTab();
  private readonly transcript = new TranscriptTab();
  private readonly notes = new NotesTab();
  private readonly playlist = new PlaylistTab();
  private readonly info = new InfoTab();

  private dragging = false;
  private currentTab: PanelTab = 'chapters';

  constructor() {
    this.handle = el('div', {
      class: 'panel-resize',
      role: 'separator',
      'aria-orientation': 'vertical',
      'aria-label': 'Resize the side panel',
      tabindex: '0',
    });

    this.titleNode = el('h2', { class: 'panel-title' }, 'In this video');

    const closeButton = el('button', {
      class: 'panel-close',
      type: 'button',
      title: 'Hide the side panel  (P)',
    }, icon('close'));
    closeButton.addEventListener('click', () => this.hide());

    const tabs = el('div', { class: 'panel-tabs', role: 'tablist' });
    for (const [key, label] of [
      ['chapters', 'Chapters'],
      ['transcript', 'Transcript'],
      ['notes', 'Notes'],
      ['playlist', 'Playlist'],
      ['info', 'Info'],
    ] as Array<[PanelTab, string]>) {
      const button = el('button', {
        class: 'panel-tab',
        type: 'button',
        role: 'tab',
        'data-tab': key,
      }, label) as HTMLButtonElement;

      button.addEventListener('click', () => store.set({ panelTab: key }));
      this.tabButtons.set(key, button);
      tabs.append(button);
    }

    this.root = el(
      'aside',
      { class: 'side-panel' },
      el('header', { class: 'panel-header' }, this.titleNode, closeButton),
      tabs,
      el('div', { class: 'panel-body' },
        this.chapters.root,
        this.transcript.root,
        this.notes.root,
        this.playlist.root,
        this.info.root,
      ),
    );
  }

  mount(): void {
    this.chapters.mount();
    this.transcript.mount();
    this.notes.mount();
    this.playlist.mount();
    this.info.mount();

    this.handle.addEventListener('pointerdown', this.onDragStart);
    this.handle.addEventListener('keydown', this.onHandleKey);

    store.subscribe((state) => this.render(state));
  }

  private hide(): void {
    store.update((state) => {
      persistSidePanel(false, state.panelWidth);
      return { panelVisible: false };
    });
  }

  private onDragStart = (event: PointerEvent): void => {
    this.dragging = true;
    this.handle.setPointerCapture(event.pointerId);
    this.handle.classList.add('dragging');

    window.addEventListener('pointermove', this.onDragMove);
    window.addEventListener('pointerup', this.onDragEnd);
    event.preventDefault();
  };

  private onDragMove = (event: PointerEvent): void => {
    if (!this.dragging) return;

    // The panel is on the right, so its width is the distance from the pointer
    // to the window's right edge.
    const width = clamp(window.innerWidth - event.clientX, MIN_WIDTH, MAX_WIDTH);
    store.set({ panelWidth: width });
  };

  private onDragEnd = (event: PointerEvent): void => {
    if (!this.dragging) return;

    this.dragging = false;
    this.handle.releasePointerCapture?.(event.pointerId);
    this.handle.classList.remove('dragging');
    window.removeEventListener('pointermove', this.onDragMove);
    window.removeEventListener('pointerup', this.onDragEnd);

    // Written once at the end of the drag rather than on every pointer move.
    const state = store.get();
    persistSidePanel(state.panelVisible, state.panelWidth);
  };

  /** Keyboard resizing, so the panel is adjustable without a pointer. */
  private onHandleKey = (event: KeyboardEvent): void => {
    const step = event.shiftKey ? 40 : 10;
    let delta = 0;
    if (event.key === 'ArrowLeft') delta = step;
    else if (event.key === 'ArrowRight') delta = -step;
    else return;

    event.preventDefault();
    event.stopPropagation();

    store.update((state) => {
      const width = clamp(state.panelWidth + delta, MIN_WIDTH, MAX_WIDTH);
      persistSidePanel(state.panelVisible, width);
      return { panelWidth: width };
    });
  };

  private render(state: AppState): void {
    this.root.hidden = !state.panelVisible;
    this.handle.hidden = !state.panelVisible;

    if (state.panelVisible) {
      this.root.style.width = `${state.panelWidth}px`;
    }

    if (state.panelTab !== this.currentTab) {
      this.currentTab = state.panelTab;
    }

    for (const [key, button] of this.tabButtons) {
      const selected = key === state.panelTab;
      toggleClass(button, 'active', selected);
      button.setAttribute('aria-selected', selected ? 'true' : 'false');
    }

    // Tabs are hidden rather than unmounted so their scroll position and
    // rendered rows survive switching back and forth.
    this.chapters.root.hidden = state.panelTab !== 'chapters';
    this.transcript.root.hidden = state.panelTab !== 'transcript';
    this.notes.root.hidden = state.panelTab !== 'notes';
    this.playlist.root.hidden = state.panelTab !== 'playlist';
    this.info.root.hidden = state.panelTab !== 'info';

    // The heading names what the panel is showing. "In this video" is wrong for
    // the queue, which is about the videos around this one.
    this.titleNode.textContent = state.panelTab === 'playlist' ? 'Playlist' : 'In this video';
  }
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
