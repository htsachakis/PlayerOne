import { store } from '../state/store';
import type { AppState } from '../state/store';
import { closeNoteComposer, commitNoteComposer } from '../services/player';
import { el } from '../util/dom';
import { formatTimePadded } from '../util/time';
import { icon } from './icons';

/**
 * The note capture overlay.
 *
 * It opens paused and prefilled when a note is already at this moment, and
 * closes on Enter or Escape. Saving with an empty box files a bookmark, which is
 * the fastest thing the feature can do and deliberately costs nothing extra.
 */
export class NoteComposer {
  readonly root: HTMLElement;

  private readonly heading: HTMLElement;
  private readonly textarea: HTMLTextAreaElement;
  private readonly star: HTMLButtonElement;

  private open = false;
  private starred = false;

  constructor() {
    this.heading = el('span', { class: 'composer-time' });

    this.textarea = el('textarea', {
      class: 'composer-input',
      rows: '3',
      placeholder: 'Type a note, or save it empty as a bookmark',
      'aria-label': 'Note text',
    }) as HTMLTextAreaElement;

    // The textarea sits inside the global shortcut guard, which ignores typing
    // entirely, so these two keys are handled here rather than in shortcuts.ts.
    this.textarea.addEventListener('keydown', (event) => {
      if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault();
        event.stopPropagation();
        void commitNoteComposer(this.textarea.value, this.starred);
        return;
      }
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        closeNoteComposer();
      }
    });

    this.star = el('button', {
      class: 'composer-star',
      type: 'button',
      title: 'Star this note',
    }, icon('starOutline')) as HTMLButtonElement;
    this.star.addEventListener('click', () => {
      this.setStarred(!this.starred);
      this.textarea.focus();
    });

    const save = el('button', { class: 'composer-save', type: 'button' }, 'Save');
    save.addEventListener('click', () => void commitNoteComposer(this.textarea.value, this.starred));

    const cancel = el('button', { class: 'composer-cancel', type: 'button' }, 'Cancel');
    cancel.addEventListener('click', () => closeNoteComposer());

    this.root = el(
      'div',
      { class: 'note-composer', role: 'dialog', 'aria-label': 'Write a note' },
      el('div', { class: 'composer-inner' },
        el('div', { class: 'composer-head' }, this.heading, this.star),
        this.textarea,
        el('div', { class: 'composer-actions' },
          el('span', { class: 'composer-hint' }, 'Enter saves · Shift+Enter for a new line · Esc cancels'),
          cancel,
          save,
        ),
      ),
    );
    this.root.hidden = true;
  }

  mount(): void {
    store.subscribe((state) => this.render(state));
  }

  private setStarred(on: boolean): void {
    this.starred = on;
    this.star.classList.toggle('active', on);
    this.star.setAttribute('aria-pressed', on ? 'true' : 'false');
    this.star.replaceChildren(icon(on ? 'star' : 'starOutline'));
  }

  private render(state: AppState): void {
    const composer = state.noteComposer;

    if (!composer) {
      this.root.hidden = true;
      this.open = false;
      return;
    }

    // Filled only as the composer opens. Doing it on every notification would
    // overwrite what is being typed five times a second.
    if (this.open) return;

    this.open = true;
    this.root.hidden = false;
    this.textarea.value = composer.text;
    this.setStarred(composer.starred);
    this.heading.textContent = composer.existing
      ? `Editing the note at ${formatTimePadded(composer.time)}`
      : `Note at ${formatTimePadded(composer.time)}`;

    this.textarea.focus();
    this.textarea.setSelectionRange(this.textarea.value.length, this.textarea.value.length);
  }
}
