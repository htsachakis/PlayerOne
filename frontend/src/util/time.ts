/**
 * The one timestamp formatter, mirroring internal/timefmt on the Go side.
 *
 * Chapters, the transcript, the seek bar and the resume prompt all use it, so
 * they can never disagree about how a time looks.
 */

/** Formats seconds as M:SS, or H:MM:SS from an hour upwards. */
export function formatTime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) seconds = 0;

  const total = Math.floor(seconds);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;

  if (h > 0) return `${h}:${pad(m)}:${pad(s)}`;
  return `${m}:${pad(s)}`;
}

/**
 * Formats seconds with a zero-padded minutes field (05:42 / 1:05:42).
 *
 * Chapter and transcript lists use this so their timestamps form a straight
 * column rather than a ragged one.
 */
export function formatTimePadded(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) seconds = 0;

  const total = Math.floor(seconds);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;

  if (h > 0) return `${h}:${pad(m)}:${pad(s)}`;
  return `${pad(m)}:${pad(s)}`;
}

function pad(value: number): string {
  return value < 10 ? `0${value}` : String(value);
}

/** Formats a byte count for the Info tab. */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '—';

  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }

  const decimals = unit === 0 || value >= 100 ? 0 : 1;
  return `${value.toFixed(decimals)} ${units[unit]}`;
}

/** Formats a bits-per-second figure for the Info tab. */
export function formatBitrate(bitsPerSecond: number): string {
  if (!Number.isFinite(bitsPerSecond) || bitsPerSecond <= 0) return '—';

  if (bitsPerSecond >= 1_000_000) return `${(bitsPerSecond / 1_000_000).toFixed(2)} Mb/s`;
  return `${Math.round(bitsPerSecond / 1000)} kb/s`;
}

/** Formats a playback rate the way the menu labels it. */
export function formatSpeed(speed: number): string {
  if (!Number.isFinite(speed) || speed <= 0) return '1x';
  const rounded = Math.round(speed * 100) / 100;
  return `${Number.isInteger(rounded) ? rounded : rounded}x`;
}

/**
 * Formats a container's creation date.
 *
 * Matroska written by download tools stores a bare YYYYMMDD, which Date cannot
 * parse, so that shape is handled explicitly before falling back to Date.
 */
export function formatDate(value: string): string {
  if (!value) return '';

  const compact = /^(\d{4})(\d{2})(\d{2})$/.exec(value.trim());
  if (compact) return `${compact[3]}/${compact[2]}/${compact[1]}`;

  const parsed = new Date(value);
  if (!Number.isNaN(parsed.getTime())) return parsed.toLocaleDateString();

  return value;
}
