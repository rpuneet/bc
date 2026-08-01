"use client";

import { useEffect, useState } from "react";

/**
 * A screenshot that moves: an autoplaying, muted, looping capture of the
 * real running app, recorded from a live daemon. The matching static
 * screenshot doubles as the video's poster (it is frame 0 of the same
 * recording), so the frame never flashes or shifts when the video mounts.
 *
 * Progressive enhancement: the server renders the static poster <img>;
 * after hydration the video takes over — unless the visitor prefers
 * reduced motion, in which case the poster simply stays.
 *
 * Assets live in public/motion/<name>-<theme>.{webm,mp4} (VP9 primary,
 * H.264 fallback — each clip is ~100–350 KB, far lighter than a GIF)
 * with posters in public/screenshots/<name>-<theme>.png.
 */
export function MotionShot({
  name,
  theme,
  alt,
  width,
  height,
  priority = false,
  ariaHidden = false,
  className = "",
}: {
  name: string;
  theme: "dark" | "light";
  alt: string;
  width: number;
  height: number;
  priority?: boolean;
  ariaHidden?: boolean;
  className?: string;
}) {
  const poster = `/screenshots/${name}-${theme}.png`;
  const [motionOk, setMotionOk] = useState(false);

  useEffect(() => {
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => setMotionOk(!mq.matches);
    update();
    mq.addEventListener("change", update);
    return () => mq.removeEventListener("change", update);
  }, []);

  if (!motionOk) {
    return (
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={poster}
        alt={alt}
        aria-hidden={ariaHidden || undefined}
        width={width}
        height={height}
        loading={priority ? "eager" : "lazy"}
        decoding="async"
        className={className}
      />
    );
  }

  return (
    <video
      autoPlay
      muted
      loop
      playsInline
      poster={poster}
      width={width}
      height={height}
      aria-label={ariaHidden ? undefined : alt}
      aria-hidden={ariaHidden || undefined}
      className={className}
    >
      <source src={`/motion/${name}-${theme}.webm`} type="video/webm" />
      <source src={`/motion/${name}-${theme}.mp4`} type="video/mp4" />
    </video>
  );
}
