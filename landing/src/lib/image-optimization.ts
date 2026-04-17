/**
 * Image Optimization Configuration & Utilities
 *
 * This module provides utilities and best practices for handling images
 * in the mycel-landing project with optimal performance and Core Web Vitals.
 */

/**
 * Image Optimization Standards for mycel-landing
 *
 * Target Metrics:
 * - All images < 200KB
 * - Largest Contentful Paint (LCP) impact minimized
 * - Cumulative Layout Shift (CLS) = 0 (use aspect ratio boxes)
 * - Interaction to Next Paint (INP) unaffected
 */

export interface ImageAsset {
  /** Original source path */
  src: string;
  /** WebP optimized version */
  webp: string;
  /** File size in bytes */
  fileSize: number;
  /** Image dimensions */
  width: number;
  height: number;
  /** Responsive sizes available */
  sizes: Array<{
    size: number;
    file: string;
    webp: string;
  }>;
}

/**
 * Standard responsive breakpoints for srcset
 * Matches Tailwind breakpoints for consistency
 */
export const RESPONSIVE_SIZES = {
  sm: 320, // Mobile
  md: 480, // Small mobile
  lg: 768, // Tablet
  xl: 1024, // iPad/Small laptop
  "2xl": 1280, // Desktop
  "3xl": 1920, // Large desktop
} as const;

/**
 * Image optimization presets
 * Use these as templates for different image types
 */
export const imagePresets = {
  /** Hero/banner images - can be larger but must optimize */
  hero: {
    maxSize: 300 * 1024, // 300KB for hero
    quality: 85,
    formats: ["webp", "jpeg"],
  },
  /** Content/article images */
  content: {
    maxSize: 150 * 1024, // 150KB
    quality: 85,
    formats: ["webp", "jpeg"],
  },
  /** Icons and small graphics - keep very small */
  icon: {
    maxSize: 30 * 1024, // 30KB
    quality: 90,
    formats: ["webp", "png"],
  },
  /** Logos - ensure crisp quality */
  logo: {
    maxSize: 50 * 1024, // 50KB
    quality: 95,
    formats: ["webp", "png"],
  },
} as const;

/**
 * Calculate image aspect ratio for CSS
 * Prevents Cumulative Layout Shift (CLS)
 *
 * Usage:
 * ```tsx
 * <div style={{ paddingBottom: getAspectRatioPadding(1920, 1080) }}>
 *   <img src="image.webp" alt="" />
 * </div>
 * ```
 */
export function getAspectRatioPadding(width: number, height: number): string {
  return `${(height / width) * 100}%`;
}

/**
 * Generate srcset string for responsive images
 *
 * Usage:
 * ```tsx
 * <img
 *   srcSet={generateSrcSet("/images/hero", ".jpg")}
 *   sizes="(max-width: 768px) 100vw, (max-width: 1280px) 50vw, 100vw"
 *   src="/images/hero-1280.jpg"
 *   alt="Hero image"
 * />
 * ```
 */
export function generateSrcSet(
  basePath: string,
  format: string,
  sizes: number[] = Object.values(RESPONSIVE_SIZES),
): string {
  return sizes
    .filter((size) => size <= 1920) // Don't exceed max desktop size
    .map((size) => `${basePath}-${size}w${format} ${size}w`)
    .join(", ");
}

/**
 * Generate picture element sources for WebP with fallback
 */
export function generatePictureSources(basePath: string, sizes?: number[]) {
  const sizesConfig = sizes || Object.values(RESPONSIVE_SIZES);
  return {
    webp: generateSrcSet(basePath, ".webp", sizesConfig),
    jpeg: generateSrcSet(basePath, ".jpg", sizesConfig),
    sizes: "(max-width: 768px) 100vw, (max-width: 1280px) 50vw, 100vw",
  };
}

/**
 * Image loading strategy helpers
 */
export const loadingStrategies = {
  /** Eager load for above-the-fold images (hero, first content image) */
  eager: {
    loading: "eager" as const,
    decoding: "sync" as const,
  },
  /** Lazy load for below-the-fold images (most content images) */
  lazy: {
    loading: "lazy" as const,
    decoding: "async" as const,
  },
  /** Auto - let browser decide based on visibility */
  auto: {
    loading: "lazy" as const,
    decoding: "async" as const,
  },
} as const;

/**
 * Validation helpers for image optimization compliance
 */
export function validateImageAsset(asset: Partial<ImageAsset>): {
  isValid: boolean;
  errors: string[];
} {
  const errors: string[] = [];

  if (!asset.src) errors.push("Missing src");
  if (!asset.webp) errors.push("Missing WebP version");
  if (!asset.fileSize) errors.push("Missing fileSize");

  if (asset.fileSize && asset.fileSize > 200 * 1024) {
    errors.push(`File size ${asset.fileSize / 1024}KB exceeds 200KB limit`);
  }

  if (!asset.sizes || asset.sizes.length === 0) {
    errors.push("Missing responsive sizes");
  }

  return {
    isValid: errors.length === 0,
    errors,
  };
}

/**
 * Core Web Vitals tracking helpers
 *
 * Use with performance observers to measure actual impact
 */
export const coreWebVitalsTargets = {
  lcp: 2500, // 2.5 seconds - Largest Contentful Paint
  fid: 100, // 100ms - First Input Delay (deprecated, use INP instead)
  inp: 200, // 200ms - Interaction to Next Paint
  cls: 0.1, // 0.1 - Cumulative Layout Shift
} as const;

/**
 * Image optimization checklist for developers
 *
 * Before adding new images:
 * ✓ Export in WebP + JPEG/PNG formats
 * ✓ Create responsive versions: 320w, 480w, 768w, 1024w, 1280w, 1920w
 * ✓ Total file size < 200KB (per format)
 * ✓ Use OptimizedImage component
 * ✓ Set aspect-ratio CSS to prevent CLS
 * ✓ Use loading="lazy" for non-hero images
 * ✓ Include descriptive alt text
 * ✓ Test with Lighthouse (target >90)
 * ✓ Verify Core Web Vitals in PageSpeed Insights
 */
export const imageOptimizationChecklist = [
  "Export in WebP + JPEG/PNG formats",
  "Create responsive versions (320w, 480w, 768w, 1024w, 1280w, 1920w)",
  "Verify file size < 200KB per format",
  "Use OptimizedImage component",
  "Set aspect-ratio CSS to prevent CLS",
  "Use loading='lazy' for non-hero images",
  "Include descriptive alt text",
  "Test with Lighthouse (target >90)",
  "Verify Core Web Vitals in PageSpeed Insights",
] as const;
