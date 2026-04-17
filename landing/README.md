# mycel Landing Page

> Official landing page for **mycel** - Multi-Agent Orchestration for Claude Code

A modern, interactive landing page showcasing mycel's capabilities for coordinating multiple AI agents with predictable behavior and cost awareness.

## 🎯 About mycel

**mycel** is a simpler, more controllable agent orchestrator for coordinating multiple Claude Code agents. It enables:

- **Hierarchical Agent System** - Organized team structure (Root, Product Manager, Manager, Tech Leads, Engineers, QA)
- **Git Worktrees** - Isolated development environments for conflict-free parallel work
- **Real-time Channels** - Instant communication between agents
- **TUI Dashboard** - Visual monitoring of agent status and progress
- **Multi-Tool Support** - Works with Claude Code, Cursor, and other AI development tools

Learn more: [bc on GitHub](https://github.com/rpuneet/bc)

## 📋 Project Structure

```
bc-landing/
├── src/
│   └── app/
│       ├── page.tsx                 # Landing page
│       ├── product/page.tsx         # Product features page
│       ├── docs/page.tsx            # Documentation page
│       ├── waitlist/page.tsx        # Waitlist signup
│       ├── layout.tsx               # Root layout
│       └── _components/
│           ├── Nav.tsx              # Navigation component
│           ├── ProductDemos.tsx     # Interactive product demos
│           ├── ProductCarouselDemos.tsx  # Carousel for demos
│           ├── BcHomeDemo.tsx       # mycel home screen demo
│           ├── UiMocks.tsx          # UI mockups
│           ├── Motion.tsx           # Animation utilities
│           └── StatsBar.tsx         # Statistics display
├── public/                          # Static assets
├── package.json                     # Dependencies
├── tsconfig.json                    # TypeScript configuration
├── next.config.ts                   # Next.js configuration
└── tailwind.config.js               # Tailwind CSS configuration
```

## 🛠 Tech Stack

- **Framework**: Next.js 16.1.6
- **Language**: TypeScript 5
- **Styling**: Tailwind CSS 4 + PostCSS
- **Animations**: Framer Motion 12.33
- **Icons**: Lucide React
- **Runtime**: React 19.2.3

## 🚀 Quick Start

### Prerequisites

- Node.js 18+ or Bun
- npm, yarn, pnpm, or bun package manager

### Installation

```bash
# Clone the repository
git clone https://github.com/bcinfra1/bc-landing.git
cd bc-landing

# Install dependencies
npm install
# or
bun install
```

### Development

```bash
# Start development server
npm run dev
# or
bun dev

# Open your browser
# http://localhost:3000
```

The application will hot-reload as you make changes.

### Production Build

```bash
# Build for production
npm run build

# Start production server
npm run start

# or for static export
npm run export
```

### Linting

```bash
# Run ESLint
npm run lint
```

## 📄 Pages

### Landing Page (`/`)
- Hero section showcasing mycel features
- Interactive product demos
- Key statistics
- Call-to-action sections
- Responsive design for all devices

### Product Page (`/product`)
- Detailed feature showcase
- Interactive component demonstrations
- Architecture overview
- Use cases and benefits

### Documentation (`/docs`)
- Comprehensive guides
- API references
- Workflow documentation
- Best practices

### Waitlist (`/waitlist`)
- Early access signup form
- Email capture
- Success confirmation

## 🎨 Key Features

### Interactive Demos
- **Product Demos**: Carousel-based demonstration of mycel features
- **mycel Home Mock**: Shows the TUI dashboard interface
- **UI Mockups**: Interactive UI component previews

### Animations
- Smooth Framer Motion animations
- Scroll-triggered effects
- Hover interactions
- Page transitions

### Responsive Design
- Mobile-first approach
- Breakpoints: 320px, 480px, 768px, 1024px, 1280px
- Touch-friendly interactions
- Optimized for all devices

### Performance
- Server-side rendering (SSR)
- Image optimization
- CSS-in-JS efficiency
- Core Web Vitals optimization

### SEO
- Meta tags and Open Graph
- Structured data
- Sitemap support
- Mobile-friendly

## 🔧 Development Workflow

### Feature Development

1. **Create a feature branch**
   ```bash
   git checkout -b feature/issue-XX-description
   ```

2. **Make your changes**
   - Follow the existing code style
   - Update components in `src/app/_components/`
   - Add new pages in `src/app/`

3. **Test locally**
   ```bash
   npm run dev
   # Test at http://localhost:3000
   ```

4. **Run linter**
   ```bash
   npm run lint
   ```

5. **Commit and push**
   ```bash
   git add .
   git commit -m "feat: description of changes"
   git push origin feature/issue-XX-description
   ```

6. **Create a Pull Request**
   - Link to the GitHub issue
   - Describe the changes
   - Reference any related PRs
   - Post to #review channel for team review

### Code Standards

- **TypeScript**: Use strict mode, proper typing
- **Components**: Functional components with hooks
- **Styling**: Tailwind CSS classes (no inline styles)
- **Naming**: PascalCase for components, camelCase for functions/variables
- **Comments**: Document complex logic and UI interactions

## 🧪 Testing & QA

### QA Test Coverage Areas

**Mobile Experience (Epic #8)**
- Responsive breakpoints: 320px, 768px, 1024px
- Touch interactions without hover states
- Font readability on small screens
- Image loading and optimization

**Product Demos (Epic #7)**
- Component rendering and interactivity
- Animation smoothness and performance
- Cross-browser compatibility
- Animation timing and responsiveness

**Content & Copy (Epic #6)**
- Copywriting accuracy and tone
- Grammar and spelling
- Brand voice consistency

**Performance & SEO (Epic #9)**
- Core Web Vitals (LCP, FID, CLS)
- Page load time < 3 seconds
- Lighthouse score > 90
- SEO metadata and structured data

**Documentation (Epic #10)**
- Link integrity and functionality
- Navigation structure
- Mobile responsiveness
- Content completeness

### Test Environment

- **Desktop**: Chrome, Safari, Firefox (latest versions)
- **Mobile**: iOS Safari, Chrome on Android
- **Devices**: iPhone SE, iPad, Android tablet
- **Performance**: Lighthouse, WebPageTest

## 📦 Build & Deployment

### Cloudflare Pages

The project is configured for deployment on Cloudflare Pages:

```bash
# Build configuration in .github/workflows or wrangler.toml
npm run build
```

### GitHub Actions

Automated CI/CD workflow:
- Linting on push
- Build verification
- Deployment on merge to main

### Environment Variables

Create a `.env.local` file for local development:

```bash
# Add any required environment variables here
```

## 📚 Resources

- [bc Documentation](https://github.com/rpuneet/bc)
- [Next.js Documentation](https://nextjs.org/docs)
- [Tailwind CSS Documentation](https://tailwindcss.com/docs)
- [Framer Motion Documentation](https://www.framer.com/motion)
- [React Documentation](https://react.dev)

## 🤝 Contributing

We welcome contributions! Please follow these steps:

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/issue-XX-description`
3. Make your changes
4. Test thoroughly
5. Submit a pull request with a clear description
6. Link to the relevant GitHub issue
7. Post to #review channel for team review

### Pull Request Guidelines

- Reference the issue number: `Fixes #XX`
- Provide a clear description of changes
- Include screenshots for UI changes
- Ensure all tests pass
- Follow code standards
- Wait for tech lead code review
- Wait for QA approval
- Merge only after CI passes

## 📊 Performance Targets

- **Lighthouse Score**: > 90
- **LCP (Largest Contentful Paint)**: < 2.5s
- **FID (First Input Delay)**: < 100ms
- **CLS (Cumulative Layout Shift)**: < 0.1
- **Page Load Time**: < 3s
- **Time to Interactive**: < 3.5s

## 🔍 Monitoring

Monitor page performance using:
- [Google PageSpeed Insights](https://pagespeed.web.dev)
- [WebPageTest](https://www.webpagetest.org)
- Chrome DevTools Lighthouse
- Core Web Vitals report in Google Search Console

## 📝 License

This project is part of the mycel ecosystem. See LICENSE for details.

## 🙋 Support

- **Issues**: [GitHub Issues](https://github.com/bcinfra1/bc-landing/issues)
- **Discussions**: [GitHub Discussions](https://github.com/bcinfra1/bc-landing/discussions)
- **Team Channels**: Join #all, #eng, #product channels in mycel workspace

---

**Last Updated**: February 2026

Made with ❤️ by the mycel team
