import type {
  Chapter,
  Diagnostics,
  MediaInfo,
  PlaybackState,
  PlaylistState,
  RecentEntry,
  Settings,
  Track,
  TranscriptResult,
  UpdateInfo,
} from '../types/media';

export type PanelTab = 'chapters' | 'transcript' | 'info' | 'playlist';

/** Which expandable drawer, if any, is open above the control bar. */
export type Drawer = 'none' | 'subtitles' | 'audio' | 'speed' | 'settings' | 'recent' | 'resume';

export interface AppState {
  /** Playback is owned by the backend; the UI only ever reflects it. */
  playback: PlaybackState;
  tracks: Track[];
  chapters: Chapter[];
  mediaInfo: MediaInfo | null;
  transcript: TranscriptResult | null;
  transcriptLoading: boolean;

  settings: Settings;
  diagnostics: Diagnostics | null;
  recent: RecentEntry[];
  playlist: PlaylistState;

  /** True once the media engine has started, or failed to. */
  engineChecked: boolean;
  engineReady: boolean;
  startupError: string;

  /** Set while a file is opening, before mpv reports it loaded. */
  opening: boolean;
  error: string | null;

  panelVisible: boolean;
  panelWidth: number;
  panelTab: PanelTab;
  drawer: Drawer;

  fullscreen: boolean;
  /** In fullscreen, controls hide after a period without mouse movement. */
  controlsHidden: boolean;

  followTranscript: boolean;
  chapterQuery: string;
  transcriptQuery: string;

  resumePrompt: { position: number; filename: string } | null;

  /** Set once a check finds a newer release; drives the update bar. */
  update: UpdateInfo | null;
  updateDismissed: boolean;
  /** Bytes downloaded so far while an update is being fetched. */
  updateProgress: { done: number; total: number } | null;
  updateInstalling: boolean;
}

export const defaultPlayback: PlaybackState = {
  position: 0,
  duration: 0,
  paused: true,
  muted: false,
  volume: 100,
  speed: 1,
  reverse: false,
  chapterIndex: -1,
  fileLoaded: false,
  path: '',
  title: '',
  idle: true,
  seeking: false,
  eof: false,
  subtitleId: 0,
  audioId: 0,
  subtitleDelay: 0,
  audioDelay: 0,
  subtitleScale: 1,
  hwdec: '',
};

export const defaultSettings: Settings = {
  volume: 100,
  muted: false,
  playbackSpeed: 1,
  autoResume: false,
  followTranscript: true,
  sidePanelVisible: true,
  sidePanelWidth: 380,
  lastSubtitleLang: '',
  lastAudioLang: '',
  subtitleDelay: 0,
  audioDelay: 0,
  subtitleScale: 1,
  checkForUpdates: true,
  skippedVersion: '',
  window: { width: 1440, height: 900, x: 0, y: 0, maximised: false, valid: false },
  logLevel: 'INFO',
};

type Listener = (state: AppState) => void;

/**
 * The single source of truth for the interface.
 *
 * Components read from here and never keep their own copy of position, paused,
 * speed or the selected tracks. That is what makes it structurally impossible
 * for two parts of the interface to disagree, rather than merely unlikely.
 */
class Store {
  private state: AppState = {
    playback: { ...defaultPlayback },
    tracks: [],
    chapters: [],
    mediaInfo: null,
    transcript: null,
    transcriptLoading: false,

    settings: { ...defaultSettings },
    diagnostics: null,
    recent: [],
    playlist: { items: [], current: -1, shuffle: false, repeat: 'off' },

    engineChecked: false,
    engineReady: false,
    startupError: '',

    opening: false,
    error: null,

    panelVisible: true,
    panelWidth: 380,
    panelTab: 'chapters',
    drawer: 'none',

    fullscreen: false,
    controlsHidden: false,

    followTranscript: true,
    chapterQuery: '',
    transcriptQuery: '',

    resumePrompt: null,

    update: null,
    updateDismissed: false,
    updateProgress: null,
    updateInstalling: false,
  };

  private listeners = new Set<Listener>();

  /** Coalesces bursts of updates into one render per animation frame. */
  private frame: number | null = null;

  get(): Readonly<AppState> {
    return this.state;
  }

  set(patch: Partial<AppState>): void {
    this.state = { ...this.state, ...patch };
    this.scheduleNotify();
  }

  /** Applies a change computed from the current state. */
  update(mutate: (state: AppState) => Partial<AppState>): void {
    this.set(mutate(this.state));
  }

  subscribe(listener: Listener): () => void {
    this.listeners.add(listener);
    listener(this.state);
    return () => this.listeners.delete(listener);
  }

  private scheduleNotify(): void {
    if (this.frame !== null) return;

    // Playback state arrives five times a second and several components react
    // to it; batching to a frame keeps that to one layout pass.
    this.frame = requestAnimationFrame(() => {
      this.frame = null;
      const snapshot = this.state;

      // Each listener is isolated. A component that throws must not stop the
      // ones registered after it: that failure mode is invisible and looks like
      // half the interface silently freezing, which is far harder to diagnose
      // than one broken panel.
      this.listeners.forEach((listener) => {
        try {
          listener(snapshot);
        } catch (err) {
          reportListenerError(err);
        }
      });
    });
  }
}

export const store = new Store();

/**
 * Reports a render failure without importing the player service, which would
 * create a cycle: the service imports the store.
 */
function reportListenerError(err: unknown): void {
  const detail = err instanceof Error && err.stack ? err.stack : String(err);
  console.error('[PlayerOne] a component failed to render', err);

  // Sent through the same channel as other client errors so it reaches the log
  // file. Guarded because the bridge may not exist during very early startup.
  try {
    const bridge = (window as { go?: Record<string, Record<string, { LogClientError?: (m: string) => void }>> }).go;
    bridge?.main?.App?.LogClientError?.(`render failure: ${detail}`);
  } catch {
    // Nothing further can be done; the console line above is the fallback.
  }
}
