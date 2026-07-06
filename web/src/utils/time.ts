// Shared time-formatting helpers for the web UI.
//
// Consolidates six divergent helpers that existed across AgentActivityStream,
// AgentDetail, liveHelpers, messageUtils, Cron, and Secrets (#3181). Every
// caller shares the same relative-time ladder, empty-value handling, and
// fallback beyond the max-days horizon.

type TimeInput = string | number | Date | null | undefined;

const DAY_MS = 86_400_000;
const HOUR_MS = 3_600_000;
const MINUTE_MS = 60_000;
const SECOND_MS = 1_000;

function toDate(input: TimeInput): Date | null {
  if (input === null || input === undefined || input === "") return null;
  const d = input instanceof Date ? input : new Date(input);
  return Number.isNaN(d.getTime()) ? null : d;
}

export interface FormatRelativeOptions {
  /** String returned when input is null / empty / invalid. Default `—`. */
  emptyLabel?: string;
  /**
   * When true, future timestamps render as `in Xs` / `in Xm` etc.
   * When false (default), future timestamps collapse to `just now`.
   */
  allowFuture?: boolean;
  /** How many days before falling back to an absolute date. Default 30. */
  maxDays?: number;
}

/**
 * Human-readable relative time: `just now` / `Xs ago` / `Xm ago` / `Xh ago`
 * / `Xd ago`, falling back to a locale date string beyond `maxDays`.
 */
export function formatRelative(input: TimeInput, opts: FormatRelativeOptions = {}): string {
  const emptyLabel = opts.emptyLabel ?? "—";
  const d = toDate(input);
  if (!d) return emptyLabel;

  const now = Date.now();
  const diff = now - d.getTime();
  const future = diff < 0;
  const absDiff = Math.abs(diff);

  if (future && opts.allowFuture) {
    return "in " + renderMagnitude(absDiff);
  }
  if (future) return "just now";

  if (absDiff < SECOND_MS) return "just now";
  const maxDays = opts.maxDays ?? 30;
  if (absDiff >= maxDays * DAY_MS) return formatAbsolute(d, { dateOnly: true });
  return renderMagnitude(absDiff) + " ago";
}

function renderMagnitude(ms: number): string {
  if (ms < MINUTE_MS) return Math.floor(ms / SECOND_MS) + "s";
  if (ms < HOUR_MS) return Math.floor(ms / MINUTE_MS) + "m";
  if (ms < DAY_MS) return Math.floor(ms / HOUR_MS) + "h";
  return Math.floor(ms / DAY_MS) + "d";
}

export interface FormatAbsoluteOptions {
  /** String returned when input is null / empty / invalid. Default `—`. */
  emptyLabel?: string;
  /** When true, only render the date (no time). Default false. */
  dateOnly?: boolean;
}

/** Locale-aware absolute timestamp, with an em-dash fallback for empty input. */
export function formatAbsolute(input: TimeInput, opts: FormatAbsoluteOptions = {}): string {
  const emptyLabel = opts.emptyLabel ?? "—";
  const d = toDate(input);
  if (!d) return emptyLabel;
  if (opts.dateOnly) {
    // Use a readable, consistent format ("Feb 5, 2026") instead of the
    // locale-default short form ("2/5/2026") so it pairs well with the
    // relative style ("3d ago", "just now") used elsewhere in the UI.
    return d.toLocaleDateString(undefined, {
      month: "short",
      day: "numeric",
      year: "numeric",
    });
  }
  return d.toLocaleString();
}

export interface FormatDurationOptions {
  /** Optional prefix, e.g. "Idle " → "Idle 5m". */
  prefix?: string;
}

/** Compact duration formatter: `Xs` / `Xm` / `Xh` / `Xd`. Negative → `0s`. */
export function formatDuration(ms: number, opts: FormatDurationOptions = {}): string {
  const prefix = opts.prefix ?? "";
  if (!Number.isFinite(ms) || ms < SECOND_MS) return prefix + "0s";
  return prefix + renderMagnitude(ms);
}
