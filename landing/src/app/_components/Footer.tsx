import Link from "next/link";

export function Footer() {
  return (
    <footer className="border-t border-border bg-accent/20">
      <div className="mx-auto max-w-7xl px-6 py-16">
        <div className="grid grid-cols-1 gap-12 md:grid-cols-5 mb-12">
          <div className="col-span-1 md:col-span-2 space-y-4">
            <div className="flex items-center group">
              <span className="font-mono text-lg font-normal text-secondary/70">
                &gt;
              </span>
              <span className="font-heading text-xl font-bold tracking-tight text-primary ml-1">
                mycel
              </span>
            </div>
            <p className="text-sm text-muted-foreground max-w-xs">
              Multi-agent orchestration for AI coding assistants. CLI-first.
              Agent-agnostic. Open source.
            </p>
          </div>
          <div className="space-y-4">
            <h2 className="text-xs font-bold uppercase tracking-widest text-primary/40">
              Product
            </h2>
            <nav
              aria-label="Product links"
              className="flex flex-col gap-2 text-sm text-muted-foreground"
            >
              <Link
                href="/"
                className="hover:text-foreground transition-colors"
              >
                Home
              </Link>
              <Link
                href="/product"
                className="hover:text-foreground transition-colors"
              >
                Features
              </Link>
              <Link
                href="/pricing"
                className="hover:text-foreground transition-colors"
              >
                Pricing
              </Link>
              <Link
                href="/method"
                className="hover:text-foreground transition-colors"
              >
                Method
              </Link>
            </nav>
          </div>
          <div className="space-y-4">
            <h2 className="text-xs font-bold uppercase tracking-widest text-primary/40">
              Resources
            </h2>
            <nav
              aria-label="Resources links"
              className="flex flex-col gap-2 text-sm text-muted-foreground"
            >
              <Link
                href="/docs"
                className="hover:text-foreground transition-colors"
              >
                Documentation
              </Link>
              <Link
                href="/docs#installation"
                className="hover:text-foreground transition-colors"
              >
                Getting Started
              </Link>
              <Link
                href="https://github.com/rpuneet/bc"
                className="hover:text-foreground transition-colors"
                target="_blank"
                rel="noopener noreferrer"
              >
                GitHub
              </Link>
            </nav>
          </div>
          <div className="space-y-4">
            <h2 className="text-xs font-bold uppercase tracking-widest text-primary/40">
              Company
            </h2>
            <nav
              aria-label="Company links"
              className="flex flex-col gap-2 text-sm text-muted-foreground"
            >
              <Link
                href="mailto:puneet@mycel.dev"
                className="hover:text-foreground transition-colors"
              >
                Contact
              </Link>
              <Link
                href="/privacy"
                className="hover:text-foreground transition-colors"
              >
                Privacy
              </Link>
              <Link
                href="/terms"
                className="hover:text-foreground transition-colors"
              >
                Terms
              </Link>
              <span className="text-muted-foreground/50 cursor-default">
                Discord
                <span className="text-[10px] ml-1 italic">(coming soon)</span>
              </span>
              <span className="text-muted-foreground/50 cursor-default">
                Twitter / X
                <span className="text-[10px] ml-1 italic">(coming soon)</span>
              </span>
            </nav>
          </div>
        </div>
        <div className="flex flex-col md:flex-row items-center justify-between gap-4 pt-8 border-t border-border text-xs text-muted-foreground/60">
          <p>&copy; {new Date().getFullYear()} mycel. All rights reserved.</p>
          <div className="flex items-center gap-6">
            <Link
              href="/privacy"
              className="hover:text-foreground transition-colors"
            >
              Privacy Policy
            </Link>
            <Link
              href="/terms"
              className="hover:text-foreground transition-colors"
            >
              Terms of Service
            </Link>
          </div>
        </div>
      </div>
    </footer>
  );
}
