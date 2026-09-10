// Shapes shared with the Go backend.
//
// These mirror the JSON tags on the Go structs. They are written by hand rather
// than taken from wailsjs/go/models.ts because the generated classes carry
// runtime constructors and `any`-typed time fields that add nothing here; these
// are plain interfaces the compiler can check properly.

export const TRACK_VIDEO = 'video';
export const TRACK_AUDIO = 'audio';
export const TRACK_SUBTITLE = 'sub';

/** The track id meaning "none selected". Real mpv track ids start at 1. */
export const NO_TRACK = 0;

export interface Chapter {
  index: number;
  title: string;
  start: number;
}

export interface Track {
  id: number;
  type: string;
  language: string;
  title: string;
  codec: string;
  selected: boolean;
  default: boolean;
  external: boolean;
  externalFilename: string;
  ffIndex: number;
  channels: number;
  sampleRate: number;
  width: number;
  height: number;
  fps: number;
  /** True for picture-based subtitles, which cannot produce a transcript. */
  imageBased: boolean;
  label: string;
}

export interface PlaybackState {
  position: number;
  duration: number;
  paused: boolean;
  muted: boolean;
  volume: number;
  speed: number;
  reverse: boolean;
  chapterIndex: number;
  fileLoaded: boolean;
  path: string;
  title: string;
  idle: boolean;
  seeking: boolean;
  eof: boolean;
  subtitleId: number;
  audioId: number;
  subtitleDelay: number;
  audioDelay: number;
  /** Multiplies the subtitle font size; 1 is mpv's own size. */
  subtitleScale: number;
  hwdec: string;
}

export interface VideoInfo {
  codec: string;
  codecLong: string;
  profile: string;
  width: number;
  height: number;
  fps: number;
  bitrate: number;
  pixelFormat: string;
}

export interface AudioInfo {
  id: number;
  codec: string;
  codecLong: string;
  language: string;
  title: string;
  channels: number;
  channelLayout: string;
  sampleRate: number;
  bitrate: number;
  label: string;
}

export interface SubtitleInfo {
  id: number;
  codec: string;
  language: string;
  title: string;
  external: boolean;
  imageBased: boolean;
  label: string;
}

export interface MediaInfo {
  path: string;
  filename: string;
  directory: string;
  fileSize: number;
  duration: number;
  container: string;
  containerLong: string;
  title: string;
  creationDate: string;
  overallBitrate: number;
  video: VideoInfo;
  hasVideo: boolean;
  audio: AudioInfo[];
  subtitles: SubtitleInfo[];
  chapters: Chapter[];
  tracks: Track[];
  videoTrackCount: number;
  audioTrackCount: number;
  subtitleTrackCount: number;
  probeError: string;
}

export interface TranscriptEntry {
  start: number;
  end: number;
  text: string;
}

export interface TranscriptResult {
  entries: TranscriptEntry[];
  trackId: number;
  trackLabel: string;
  source: string;
  available: boolean;
  /** Always populated, so an empty transcript can explain itself. */
  status: string;
}

export interface WindowState {
  width: number;
  height: number;
  x: number;
  y: number;
  maximised: boolean;
  valid: boolean;
}

export interface Settings {
  volume: number;
  muted: boolean;
  playbackSpeed: number;
  autoResume: boolean;
  followTranscript: boolean;
  sidePanelVisible: boolean;
  sidePanelWidth: number;
  lastSubtitleLang: string;
  lastAudioLang: string;
  subtitleDelay: number;
  audioDelay: number;
  subtitleScale: number;
  checkForUpdates: boolean;
  lastUpdateCheck: number;
  skippedVersion: string;
  window: WindowState;
  logLevel: string;
}

export interface OpenResult {
  path: string;
  filename: string;
  resumeAvailable: boolean;
  resumePosition: number;
  autoResumed: boolean;
}

export interface Diagnostics {
  appName: string;
  tagline: string;
  engineReady: boolean;
  startupError: string;
  mpvPath: string;
  ffmpegPath: string;
  ffprobePath: string;
  searchDirs: string[];
  settingsPath: string;
  historyPath: string;
  hwdec: string;
}

export interface RecentEntry {
  path: string;
  filename: string;
  title: string;
  position: number;
  duration: number;
  updatedAt: string;
  exists: boolean;
}

export interface TracksPayload {
  tracks: Track[];
  chapters: Chapter[];
}

/** How the queue behaves when a file, or the whole queue, ends. */
export type RepeatMode = 'off' | 'all' | 'one';

export interface PlaylistItem {
  path: string;
  filename: string;
  title: string;
  duration: number;
  /** True when the file is no longer on disk. */
  missing: boolean;
}

export interface PlaylistState {
  items: PlaylistItem[];
  /** Index of the entry that is playing, or -1 when the queue is empty. */
  current: number;
  shuffle: boolean;
  repeat: RepeatMode;
}

// --- Updates ---

export interface UpdateRelease {
  version: string;
  name: string;
  notes: string;
  url: string;
  publishedAt: string;
  installerUrl: string;
  installerName: string;
  installerSize: number;
  /** False when the release published no SHA256SUMS.txt to verify against. */
  hasChecksums: boolean;
}

export interface UpdateStatus {
  checked: boolean;
  available: boolean;
  current: string;
  latest: UpdateRelease | null;
  /** Always populated, including when no update was offered and why. */
  message: string;
}

export interface UpdateInfo {
  version: string;
  detail: string;
  /** False for the portable copy, which has no installer to update in place. */
  installed: boolean;
  status: UpdateStatus;
}
