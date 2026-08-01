import { MotionShot } from "./MotionShot";

/* Framed product screenshot with window chrome.
 *
 * Screenshots are real captures of the running app, checked into
 * public/screenshots/. Plain <img> is intentional: the site is statically
 * exported with unoptimized images, and the files are pre-compressed PNGs.
 * When a light variant exists, both are rendered and CSS picks the one that
 * matches the visitor's theme (.shot-dark / .shot-light in globals.css).
 *
 * Pass `motion` to play a live recording of the view instead (MotionShot):
 * the static screenshot stays as the poster and as the reduced-motion
 * fallback, so the frame reads identically either way.
 */

export function ProductFrame({
  srcDark,
  srcLight,
  motion,
  alt,
  title,
  width,
  height,
  priority = false,
  className = "",
}: {
  srcDark: string;
  srcLight?: string;
  /** Base name of a clip in public/motion/ (e.g. "apps" for apps-dark). */
  motion?: string;
  alt: string;
  title?: string;
  width: number;
  height: number;
  priority?: boolean;
  className?: string;
}) {
  const loading = priority ? "eager" : "lazy";
  return (
    <figure className={`product-frame ${className}`}>
      <div className="product-frame-bar" aria-hidden="true">
        <span className="product-frame-dot" />
        <span className="product-frame-dot" />
        <span className="product-frame-dot" />
        {title && <span className="product-frame-title">{title}</span>}
      </div>
      {motion ? (
        <>
          <MotionShot
            name={motion}
            theme="dark"
            alt={alt}
            width={width}
            height={height}
            priority={priority}
            className={srcLight ? "shot-dark" : undefined}
          />
          {srcLight && (
            <MotionShot
              name={motion}
              theme="light"
              alt=""
              ariaHidden
              width={width}
              height={height}
              className="shot-light"
            />
          )}
        </>
      ) : (
        <>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={srcDark}
            alt={alt}
            width={width}
            height={height}
            loading={loading}
            decoding="async"
            className={srcLight ? "shot-dark" : undefined}
          />
          {srcLight && (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={srcLight}
              alt=""
              aria-hidden="true"
              width={width}
              height={height}
              loading={loading}
              decoding="async"
              className="shot-light"
            />
          )}
        </>
      )}
    </figure>
  );
}
