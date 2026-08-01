# mycel Landing Page

Official landing page for **mycel** — CLI-first multi-agent orchestration for Claude, Gemini, Cursor, and other AI coding agents. Lives at `landing/` inside the [rpuneet/mycel](https://github.com/rpuneet/mycel) monorepo and deploys to bc-infra.com.

## Tech Stack

- **Framework**: Next.js 16
- **Language**: TypeScript
- **Styling**: Tailwind CSS 4
- **Animations**: Framer Motion
- **Runtime**: React 19

## Project Structure

```
landing/
├── src/app/
│   ├── page.tsx              # Landing page
│   ├── docs/page.tsx         # Docs page
│   ├── waitlist/page.tsx     # Waitlist signup
│   ├── privacy/, terms/      # Legal pages
│   ├── layout.tsx            # Root layout
│   └── _components/          # Nav, hero, install section, changelog, etc.
├── public/                   # Static assets
├── package.json
├── tsconfig.json
├── next.config.ts
└── tailwind.config.js
```

## Quick Start

From the repo root:

```bash
make run-landing               # Dev server with hot reload → http://localhost:3000
make build-local-landing       # Production build
make test-landing              # Run tests
make lint-ts                   # Lint (includes landing)
```

Or directly with Bun from `landing/`:

```bash
bun install
bun run dev      # http://localhost:3000
bun run build
bun run lint
```

## Deployment

Deploys to Cloudflare Pages (bc-infra.com) from CI on merge to `main`. See `.github/workflows/` for the deploy pipeline.

## Contributing

Follow the repo-wide [CONTRIBUTING.md](../CONTRIBUTING.md) for branch naming, commit conventions, and the PR process. UI changes should include a description of what was changed and, where useful, before/after screenshots.

## Resources

- [mycel on GitHub](https://github.com/rpuneet/mycel)
- [Next.js Documentation](https://nextjs.org/docs)
- [Tailwind CSS Documentation](https://tailwindcss.com/docs)
- [Framer Motion Documentation](https://www.framer.com/motion)
