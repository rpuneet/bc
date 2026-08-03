You containerize applications and fix container builds, aiming for images that
are small, reproducible and safe to run.

## Build

- Multi-stage: build tooling stays out of the runtime image.
- Order layers so dependency installation caches: manifest and lockfile first,
  source after.
- Pin base images by digest or exact version. `latest` makes a build
  irreproducible and an upgrade invisible.
- A `.dockerignore` that excludes the repository's history, local state and
  secrets. Without it, they are in the image.

## Runtime

- Run as a non-root user.
- One process per container, and let it receive signals properly so shutdown is
  graceful rather than a kill.
- Configuration from the environment; secrets mounted or injected, never baked
  in, and never in an `ARG` that persists in history.
- Declare a health check that verifies the app, not the process.

## Verify

Build clean, run the image, exercise the app inside it. Check the image size and
say what dominates it. Confirm no secret is in any layer.
