import * as App from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';

import { store, defaultSettings } from '../state/store';
import type {
  Diagnostics,
  MediaInfo,
  Note,
  NotesResult,
  OpenResult,
  PlaybackState,
  PlaylistState,
  RecentEntry,
  Settings,
  RepeatMode,
  TracksPayload,
  TranscriptResult,
  UpdateInfo,
} from '../types/media';

/**
 * The only place the frontend talks to Go.
 *
 * Every call routes through here so that error handling, state updates and
 * event wiring live in one file rather than being repeated in each component.
 */

/**
 * Describes an unknown thrown value in a way a person can act on.
 *
 * Wails rejects with a plain string, the DOM throws Errors, and a bug can throw
 * anything at all. A blank banner is worse than a clumsy one, so every branch
 * ends with something readable.
 */
function describe(err: unknown): string {
  if (typeof err === 'string') return err.trim() || 'The operation failed without giving a reason.';
  if (err instanceof Error) return err.message.trim() || `${err.name} (no message)`;

  if (err && typeof err === 'object') {
    const message = (err as { message?: unknown }).message;
    if (typeof message === 'string' && message.trim()) return message.trim();
    try {
      const json = JSON.stringify(err);
      if (json && json !== '{}') return json;
    } catch {
      // A value that cannot be serialised still needs a message.
    }
  }

  return 'Something went wrong.';
}

/** Sends a message to the backend log, where it survives the window closing. */
export function logToBackend(message: string, level: 'error' | 'info' = 'error'): void {
  try {
    if (level === 'error') void App.LogClientError(message);
    else void App.LogClientInfo(message);
  } catch {
    // The bridge may not be ready during very early startup; losing one log
    // line must never turn into a second failure.
  }
}

/** Shows a message to the user and records the underlying error. */
export function reportError(err: unknown): void {
  const message = describe(err);
  console.error('[PlayerOne]', err);

  const detail = err instanceof Error && err.stack ? `${message}\n${err.stack}` : message;
  logToBackend(detail);

  store.set({ error: message });
}

export function clearError(): void {
  store.set({ error: null });
}

/** Runs a backend call, surfacing any failure as a user-visible message. */
async function guard<T>(work: () => Promise<T>): Promise<T | undefined> {
  try {
    return await work();
  } catch (err) {
    reportError(err);
    return undefined;
  }
}

// --- Playback ---

export const playPause = () => guard(() => App.PlayPause());
export const play = () => guard(() => App.Play());
export const pause = () => guard(() => App.Pause());
export const stop = () => guard(() => App.Stop());
export const seek = (seconds: number) => guard(() => App.Seek(seconds));
export const seekRelative = (delta: number) => guard(() => App.SeekRelative(delta));
export const seekChapter = (index: number) => guard(() => App.SeekChapter(index));
export const setVolume = (value: number) => guard(() => App.SetVolume(value));
export const setMute = (muted: boolean) => guard(() => App.SetMute(muted));
export const toggleMute = () => guard(() => App.ToggleMute());
export const setSpeed = (speed: number) => guard(() => App.SetSpeed(speed));
export const setReverse = (reverse: boolean) => guard(() => App.SetReverse(reverse));
export const toggleReverse = () => guard(() => App.ToggleReverse());
export const setSubtitleTrack = (id: number) => guard(() => App.SetSubtitleTrack(id));
export const disableSubtitles = () => guard(() => App.DisableSubtitles());
export const setAudioTrack = (id: number) => guard(() => App.SetAudioTrack(id));
export const setSubtitleDelay = (seconds: number) => guard(() => App.SetSubtitleDelay(seconds));
export const setAudioDelay = (seconds: number) => guard(() => App.SetAudioDelay(seconds));
export const setSubtitleScale = (scale: number) => guard(() => App.SetSubtitleScale(scale));

/** The range the subtitle size control offers, matching the backend's clamp. */
export const SUBTITLE_SCALE_MIN = 0.25;
export const SUBTITLE_SCALE_MAX = 4;

export async function speedPresets(): Promise<number[]> {
  const presets = await guard(() => App.SpeedPresets());
  return presets ?? [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2];
}

// --- Opening media ---

export async function chooseAndOpen(): Promise<void> {
  const path = await guard(() => App.ChooseFile());
  if (path) await openPath(path);
}

export async function openPath(path: string): Promise<void> {
  clearError();
  store.set({ opening: true, resumePrompt: null, transcript: null });

  try {
    const result = (await App.Open(path)) as OpenResult;

    if (result.resumeAvailable) {
      // Playback is held until the viewer chooses, so a resume decision is
      // never made while the video is already running past the point.
      await App.Pause();
      store.set({
        resumePrompt: { position: result.resumePosition, filename: result.filename },
        drawer: 'resume',
      });
    } else {
      await App.Play();
    }

    await refreshRecent();
  } catch (err) {
    store.set({ opening: false });
    reportError(err);
    return;
  }

  store.set({ opening: false });
}

export async function acceptResume(position: number): Promise<void> {
  store.set({ resumePrompt: null, drawer: 'none' });
  await seek(position);
  await play();
}

export async function declineResume(): Promise<void> {
  store.set({ resumePrompt: null, drawer: 'none' });
  await seek(0);
  await play();
}

export async function chooseAndAddSubtitle(): Promise<void> {
  const path = await guard(() => App.ChooseSubtitle());
  if (!path) return;

  await guard(() => App.AddSubtitle(path));
  await refreshTranscript();
}

export const handleDrop = (paths: string[]) => guard(() => App.HandleDrop(paths));

// --- Panels and data ---

export async function refreshTranscript(): Promise<void> {
  store.set({ transcriptLoading: true });
  try {
    const result = (await App.Transcript()) as TranscriptResult;
    store.set({ transcript: result, transcriptLoading: false });
  } catch (err) {
    store.set({ transcriptLoading: false });
    reportError(err);
  }
}

export async function refreshMediaInfo(): Promise<void> {
  const info = (await guard(() => App.MediaInfo())) as MediaInfo | null | undefined;
  store.set({ mediaInfo: info ?? null });
}

export async function refreshRecent(): Promise<void> {
  const recent = (await guard(() => App.RecentFiles())) as RecentEntry[] | undefined;
  store.set({ recent: recent ?? [] });
}

export const forgetRecent = async (path: string) => {
  await guard(() => App.ForgetRecent(path));
  await refreshRecent();
};

export const clearRecent = async () => {
  await guard(() => App.ClearRecent());
  await refreshRecent();
};

// --- Playlist ---

function applyPlaylist(state: PlaylistState | undefined): void {
  if (state) store.set({ playlist: state });
}

export async function refreshPlaylist(): Promise<void> {
  applyPlaylist((await guard(() => App.Playlist())) as PlaylistState | undefined);
}

export async function addFilesToPlaylist(): Promise<void> {
  const paths = (await guard(() => App.ChooseFiles())) as string[] | undefined;
  if (!paths || paths.length === 0) return;
  applyPlaylist((await guard(() => App.AddToPlaylist(paths))) as PlaylistState | undefined);
}

export async function addFolderToPlaylist(): Promise<void> {
  const dir = (await guard(() => App.ChooseFolder())) as string | undefined;
  if (!dir) return;
  applyPlaylist((await guard(() => App.AddToPlaylist([dir]))) as PlaylistState | undefined);
}

export async function playPlaylistItem(index: number): Promise<void> {
  applyPlaylist((await guard(() => App.PlayPlaylistItem(index))) as PlaylistState | undefined);
}

export async function removeFromPlaylist(index: number): Promise<void> {
  applyPlaylist((await guard(() => App.RemoveFromPlaylist(index))) as PlaylistState | undefined);
}

export async function clearPlaylist(): Promise<void> {
  applyPlaylist((await guard(() => App.ClearPlaylist())) as PlaylistState | undefined);
}

// --- Notes ---

/** Applies a notes result from the backend, which is always the whole list. */
function applyNotes(result: NotesResult | undefined): void {
  if (!result) return;
  store.set({ notes: result });
}

export async function refreshNotes(): Promise<void> {
  applyNotes((await guard(() => App.Notes())) as NotesResult | undefined);
}

export async function addNote(time: number, text: string, starred: boolean): Promise<void> {
  applyNotes((await guard(() => App.AddNote(time, text, starred))) as NotesResult | undefined);
}

export async function deleteNote(time: number): Promise<void> {
  applyNotes((await guard(() => App.DeleteNote(time))) as NotesResult | undefined);
}

export async function toggleNoteStar(time: number): Promise<void> {
  applyNotes((await guard(() => App.ToggleNoteStar(time))) as NotesResult | undefined);
}

export async function saveNotesAs(): Promise<void> {
  applyNotes((await guard(() => App.SaveNotesAs())) as NotesResult | undefined);
}

export async function loadNotesFrom(): Promise<void> {
  applyNotes((await guard(() => App.LoadNotesFrom())) as NotesResult | undefined);
}

/**
 * Opens the note composer for the current moment.
 *
 * The stamp is set a few seconds back because a moment is recognised as worth
 * noting only after it has passed. When a note is already there, this edits it
 * rather than filing a second one a fraction of a second away.
 */
export async function openNoteComposer(): Promise<void> {
  const state = store.get();
  if (!state.playback.fileLoaded) return;
  if (state.noteComposer) return;

  const offset = state.settings.noteCaptureOffset ?? 0;
  const time = Math.max(0, Math.round(state.playback.position - offset));

  const wasPlaying = !state.playback.paused;
  if (state.settings.pauseWhileComposingNote && wasPlaying) {
    await pause();
  }

  // A negative time is the backend's way of saying there is no note here.
  const found = (await guard(() => App.NoteAt(time))) as Note | undefined;
  const existing = found && found.time >= 0 ? found : null;

  store.set({
    noteComposer: {
      time: existing ? existing.time : time,
      text: existing?.text ?? '',
      starred: existing?.starred ?? false,
      existing: existing !== null,
      wasPlaying,
    },
  });
}

/** Closes the composer, resuming playback if it was running when it opened. */
export function closeNoteComposer(): void {
  const composer = store.get().noteComposer;
  store.set({ noteComposer: null });

  if (composer?.wasPlaying) void play();
}

export async function commitNoteComposer(text: string, starred: boolean): Promise<void> {
  const composer = store.get().noteComposer;
  if (!composer) return;

  await addNote(composer.time, text, starred);
  closeNoteComposer();
}

// --- Playlist files ---

/** Saves the queue to a file the user picks. */
export async function exportPlaylist(): Promise<string | undefined> {
  return (await guard(() => App.ExportPlaylist())) as string | undefined;
}

/** Opens a playlist file the user picks and starts playing it. */
export async function importPlaylist(): Promise<void> {
  applyPlaylist((await guard(() => App.ImportPlaylist())) as PlaylistState | undefined);
}

export const nextTrack = () => guard(() => App.NextTrack());
export const previousTrack = () => guard(() => App.PreviousTrack());

export async function setShuffle(on: boolean): Promise<void> {
  applyPlaylist((await guard(() => App.SetShuffle(on))) as PlaylistState | undefined);
}

export async function setRepeat(mode: RepeatMode): Promise<void> {
  applyPlaylist((await guard(() => App.SetRepeat(mode))) as PlaylistState | undefined);
}

/** Steps off -> all -> one -> off, which is the order every player uses. */
export function cycleRepeat(current: RepeatMode): RepeatMode {
  if (current === 'off') return 'all';
  if (current === 'all') return 'one';
  return 'off';
}

// --- Updates ---

/**
 * Asks the backend to check GitHub.
 *
 * force is set when the user asks explicitly, which bypasses both the setting
 * that turns checking off and any version they previously chose to skip.
 */
export async function checkForUpdates(force: boolean): Promise<UpdateInfo | undefined> {
  const info = (await guard(() => App.CheckForUpdates(force))) as UpdateInfo | undefined;
  if (info) {
    store.set({
      update: info,
      // An explicit check should surface its answer even if the bar was
      // dismissed earlier in the session.
      updateDismissed: force ? false : store.get().updateDismissed,
    });
  }
  return info;
}

export async function installUpdate(): Promise<void> {
  store.set({ updateInstalling: true, updateProgress: { done: 0, total: 0 } });

  try {
    await App.InstallUpdate();
    // No success path to render: the application is about to quit so the
    // installer can replace it.
  } catch (err) {
    store.set({ updateInstalling: false, updateProgress: null });
    reportError(err);
  }
}

export const openReleasePage = () => guard(() => App.OpenReleasePage());

export function dismissUpdate(): void {
  store.set({ updateDismissed: true });
}

export function skipUpdate(version: string): void {
  store.set({ updateDismissed: true });
  void guard(() => App.SkipUpdate(version));
}

export function setCheckForUpdates(on: boolean): void {
  store.update((state) => ({ settings: { ...state.settings, checkForUpdates: on } }));
  void guard(() => App.SetCheckForUpdates(on));
}

export const appVersion = () => guard(() => App.Version()) as Promise<UpdateInfo | undefined>;

// --- Settings ---

export async function loadSettings(): Promise<Settings> {
  const loaded = (await guard(() => App.Settings())) as Settings | undefined;
  const settings = loaded ?? { ...defaultSettings };

  store.set({
    settings,
    panelVisible: settings.sidePanelVisible,
    panelWidth: settings.sidePanelWidth,
    followTranscript: settings.followTranscript,
  });
  return settings;
}

export async function saveSettings(next: Settings): Promise<void> {
  const applied = (await guard(() => App.SaveSettings(next as never))) as Settings | undefined;
  if (applied) store.set({ settings: applied });
}

export function setFollowTranscript(follow: boolean): void {
  store.set({ followTranscript: follow });
  void guard(() => App.SetFollowTranscript(follow));
}

export function setAutoResume(auto: boolean): void {
  store.update((state) => ({ settings: { ...state.settings, autoResume: auto } }));
  void guard(() => App.SetAutoResume(auto));
}

export function persistSidePanel(visible: boolean, width: number): void {
  void guard(() => App.SetSidePanel(visible, Math.round(width)));
}

// --- The video surface ---

/**
 * Positions the native video window over the slot the layout reserves for it.
 *
 * The rectangle is in CSS pixels relative to the viewport, which is the same
 * origin as the window's client area; the device pixel ratio is passed so the
 * backend can convert to the physical pixels Win32 works in.
 */
export function setVideoBounds(rect: DOMRect): void {
  void App.SetVideoBounds(rect.left, rect.top, rect.width, rect.height, window.devicePixelRatio || 1);
}

export function setVideoVisible(visible: boolean): void {
  void App.SetVideoVisible(visible);
}

export function setFullscreen(full: boolean): void {
  store.set({ fullscreen: full, controlsHidden: false });
  void App.SetFullscreen(full);
}

export const diagnostics = () => guard(() => App.Diagnostics()) as Promise<Diagnostics | undefined>;
export const toolStatus = () => guard(() => App.ToolStatus()) as Promise<string[] | undefined>;

/** Callbacks woken by pointer movement the page itself cannot observe. */
const pointerListeners = new Set<() => void>();

/** Registers a callback for pointer movement reported by the backend. */
export function onPointerMoved(listener: () => void): void {
  pointerListeners.add(listener);
}

// --- Backend events ---

/**
 * Subscribes to everything the backend pushes.
 *
 * Called once at startup. The backend is authoritative: user actions call into
 * Go and the resulting state arrives here, rather than the interface predicting
 * what will happen.
 */
export function listen(): void {
  // The first state event is traced, because "the interface is not updating"
  // and "the backend is not sending" look identical from the outside.
  let tracedFirstState = false;

  EventsOn('playback:state', (state: PlaybackState) => {
    if (!tracedFirstState) {
      tracedFirstState = true;
      logToBackend(
        `first playback state: fileLoaded=${state?.fileLoaded} idle=${state?.idle} ` +
          `paused=${state?.paused} duration=${state?.duration} path=${state?.path}`,
        'info',
      );
    }
    store.set({ playback: state });
  });

  EventsOn('player:tracks', (payload: TracksPayload) => {
    store.set({
      tracks: payload?.tracks ?? [],
      chapters: payload?.chapters ?? [],
    });
  });

  EventsOn('media:opened', (info: MediaInfo | null) => {
    store.set({ mediaInfo: info ?? null, opening: false });

    // The transcript follows the selected subtitle track, which is only known
    // once the file is open.
    if (info) void refreshTranscript();
    else store.set({ transcript: null });

    // A new video must not inherit the previous one's search or half-typed note.
    store.set({ notesQuery: '', notesStarredOnly: false, noteComposer: null });

    // Asked for rather than waited for: the backend emits notes:changed as it
    // binds the file, which can happen before this listener exists when a path
    // is given on the command line.
    void refreshNotes();
  });

  EventsOn('notes:changed', (result: NotesResult) => {
    store.set({ notes: result });
  });

  EventsOn('media:ended', () => {
    void refreshRecent();
  });

  EventsOn('playlist:changed', (state: PlaylistState) => {
    applyPlaylist(state);
  });

  // A file dropped onto the video is opened by mpv, not through openPath, so
  // the resume choice is offered by the backend instead.
  EventsOn('media:resume', (prompt: { position: number; filename: string }) => {
    if (!prompt) return;
    store.set({
      resumePrompt: { position: prompt.position, filename: prompt.filename },
      drawer: 'resume',
    });
  });

  // The page cannot see the pointer while it is over the video, so the backend
  // reports movement from Windows itself. Without this the fullscreen controls
  // hide and never come back.
  EventsOn('ui:pointer-moved', () => {
    pointerListeners.forEach((listener) => listener());
  });

  EventsOn('update:available', (info: UpdateInfo) => {
    store.set({ update: info, updateDismissed: false });
  });

  EventsOn('update:progress', (progress: { done: number; total: number }) => {
    store.set({ updateProgress: progress });
  });

  EventsOn('app:error', (message: string) => {
    store.set({ error: message });
  });

  EventsOn('app:ready', (diag: Diagnostics) => {
    logToBackend(`engine ready event: ready=${diag?.engineReady} error=${diag?.startupError}`, 'info');
    store.set({
      diagnostics: diag,
      engineChecked: true,
      engineReady: diag?.engineReady ?? false,
      startupError: diag?.startupError ?? '',
    });
  });
}

/**
 * Reloads the transcript when the selected subtitle track changes.
 *
 * Watching the state rather than hooking every call site means a track change
 * from any source - a menu, a shortcut, mpv's own default selection - keeps the
 * transcript in step.
 */
export function watchSubtitleTrack(): void {
  let lastTrack = -1;
  let lastPath = '';

  store.subscribe((state) => {
    const { subtitleId, path, fileLoaded } = state.playback;
    if (!fileLoaded) {
      lastTrack = -1;
      lastPath = '';
      return;
    }
    if (subtitleId === lastTrack && path === lastPath) return;

    lastTrack = subtitleId;
    lastPath = path;
    void refreshTranscript();
  });
}
