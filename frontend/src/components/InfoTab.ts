import { store } from '../state/store';
import type { AppState } from '../state/store';
import { clear, el } from '../util/dom';
import { formatBitrate, formatBytes, formatDate, formatTime } from '../util/time';
import type { MediaInfo } from '../types/media';

/**
 * The Info tab: what this file actually is.
 *
 * Every value is labelled and formatted for a reader. Raw field names and
 * unlabelled numbers are deliberately absent, and anything unknown is omitted
 * rather than shown as an empty row.
 */
export class InfoTab {
  readonly root: HTMLElement;
  private signature = '';

  constructor() {
    this.root = el('div', { class: 'panel-tab-content info-tab' });
  }

  mount(): void {
    store.subscribe((state) => this.render(state));
  }

  private render(state: AppState): void {
    const info = state.mediaInfo;
    const signature = `${info?.path ?? ''}|${info?.duration ?? 0}|${state.playback.hwdec}|${state.diagnostics?.mpvPath ?? ''}`;
    if (signature === this.signature) return;
    this.signature = signature;

    clear(this.root);

    if (!info) {
      this.root.append(
        el('div', { class: 'panel-empty' },
          el('p', { class: 'panel-empty-title' }, 'No file open'),
          el('p', { class: 'panel-empty-detail' }, 'Open a video to see its details here.'),
        ),
      );
      this.appendEnvironment(state);
      return;
    }

    this.root.append(
      section('File', [
        row('Name', info.filename),
        row('Folder', info.directory),
        row('Size', formatBytes(info.fileSize)),
        row('Container', containerLabel(info)),
        row('Duration', formatTime(info.duration)),
        row('Overall bitrate', info.overallBitrate > 0 ? formatBitrate(info.overallBitrate) : ''),
        row('Title', info.title !== info.filename ? info.title : ''),
        row('Created', formatDate(info.creationDate)),
      ]),
    );

    if (info.hasVideo) {
      this.root.append(
        section('Video', [
          row('Codec', codecLabel(info.video.codec, info.video.codecLong)),
          row('Profile', info.video.profile),
          row('Resolution', info.video.width > 0 ? `${info.video.width} × ${info.video.height}` : ''),
          row('Frame rate', info.video.fps > 0 ? `${info.video.fps.toFixed(3).replace(/\.?0+$/, '')} fps` : ''),
          row('Bitrate', info.video.bitrate > 0 ? formatBitrate(info.video.bitrate) : ''),
          row('Pixel format', info.video.pixelFormat),
          row('Decoder', decoderLabel(state.playback.hwdec)),
        ]),
      );
    }

    const audioTracks = info.audio ?? [];
    if (audioTracks.length > 0) {
      const rows = audioTracks.flatMap((audio, index) => {
        const prefix = audioTracks.length > 1 ? `Track ${index + 1}` : 'Track';
        return [
          row(prefix, audio.label),
          row('  Codec', codecLabel(audio.codec, audio.codecLong)),
          row('  Channels', audio.channelLayout || (audio.channels > 0 ? String(audio.channels) : '')),
          row('  Sample rate', audio.sampleRate > 0 ? `${(audio.sampleRate / 1000).toFixed(1)} kHz` : ''),
          row('  Bitrate', audio.bitrate > 0 ? formatBitrate(audio.bitrate) : ''),
        ];
      });
      this.root.append(section('Audio', rows));
    }

    const subtitleTracks = info.subtitles ?? [];
    if (subtitleTracks.length > 0) {
      const rows = subtitleTracks.map((sub, index) =>
        row(`Track ${index + 1}`, sub.label + (sub.imageBased ? ' — no transcript possible' : '')),
      );
      this.root.append(section('Subtitles', rows));
    }

    this.root.append(
      section('Tracks', [
        row('Video', String(info.videoTrackCount)),
        row('Audio', String(info.audioTrackCount)),
        row('Subtitle', String(info.subtitleTrackCount)),
        row('Chapters', String((info.chapters ?? []).length)),
      ]),
    );

    if (info.probeError) {
      this.root.append(
        section('Note', [
          el('p', { class: 'info-note' },
            `Some details are unavailable: ${info.probeError}`) as HTMLElement,
        ]),
      );
    }

    this.appendEnvironment(state);
  }

  /** The environment section is what a troubleshooting request needs first. */
  private appendEnvironment(state: AppState): void {
    const diag = state.diagnostics;
    if (!diag) return;

    this.root.append(
      section('PlayerOne', [
        row('Media engine', diag.mpvPath || 'not found'),
        row('ffmpeg', diag.ffmpegPath || 'not found — transcripts from embedded subtitles are unavailable'),
        row('ffprobe', diag.ffprobePath || 'not found — some file details are unavailable'),
        row('Settings', diag.settingsPath),
        row('History', diag.historyPath),
      ]),
    );
  }
}

function containerLabel(info: MediaInfo): string {
  if (info.containerLong && info.container) return `${info.containerLong} (${info.container})`;
  return info.containerLong || info.container;
}

function codecLabel(short: string, long: string): string {
  if (!short) return long;
  if (!long || long === short) return short;
  return `${short} — ${long}`;
}

function decoderLabel(hwdec: string): string {
  if (!hwdec) return '';
  if (hwdec === 'no') return 'Software (CPU)';
  return `Hardware — ${hwdec}`;
}

/** Builds a titled section, skipping rows with no value. */
function section(title: string, rows: Array<HTMLElement | null>): HTMLElement {
  const populated = rows.filter((r): r is HTMLElement => r !== null);
  if (populated.length === 0) return el('div');

  return el('section', { class: 'info-section' },
    el('h4', { class: 'info-heading' }, title),
    el('dl', { class: 'info-rows' }, ...populated),
  );
}

/** One labelled value, or null when there is nothing to show. */
function row(label: string, value: string): HTMLElement | null {
  if (!value) return null;

  // A leading double space marks a sub-row belonging to the entry above it.
  const indented = label.startsWith('  ');
  return el('div', { class: indented ? 'info-row indented' : 'info-row' },
    el('dt', {}, label.trim()),
    el('dd', { title: value }, value),
  );
}
