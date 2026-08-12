/**
 * Resolve mycel design tokens for Tailwind color utilities.
 *
 * Tailwind 3 cannot alpha-modify bare `var(--token)` colors — utilities like
 * `border-mycel-border/70` otherwise fall back to the default gray-200 border
 * (`#e5e7eb`), which reads as a harsh white hairline on the espresso dark
 * theme. Numeric `/NN` modifiers become `color-mix`. Solid utilities go
 * through Tailwind's `--tw-*-opacity` pipeline (opacityValue is a `var(...)`);
 * leave those as the bare token.
 */
export function resolveMycelColor(cssVar: string, opacityValue?: string): string {
  if (opacityValue == null || !/^\d*\.?\d+$/.test(opacityValue)) {
    return `var(${cssVar})`;
  }
  const pct = Math.round(Number(opacityValue) * 1000) / 10;
  return `color-mix(in srgb, var(${cssVar}) ${pct}%, transparent)`;
}

/** Tailwind theme color factory — see resolveMycelColor. */
export function mycelColor(cssVar: string) {
  return ({ opacityValue }: { opacityValue?: string }) =>
    resolveMycelColor(cssVar, opacityValue);
}
