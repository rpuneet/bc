/** 8 vibrant colors that work well on dark backgrounds. */
export const AVATAR_COLORS: string[] = [
  "#6366f1", // indigo
  "#ec4899", // pink
  "#14b8a6", // teal
  "#f97316", // orange
  "#8b5cf6", // violet
  "#06b6d4", // cyan
  "#eab308", // yellow
  "#22c55e", // green
];

/** djb2 hash → color index (0-7). */
export function colorFromName(name: string): number {
  let hash = 5381;
  for (let i = 0; i < name.length; i++) {
    hash = ((hash << 5) + hash + name.charCodeAt(i)) | 0;
  }
  return Math.abs(hash) % AVATAR_COLORS.length;
}
