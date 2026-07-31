import Link from "next/link";
import { SporeLogo } from "./SporeLogo";

export function Footer() {
  return (
    <footer className="bg-surface-container-lowest">
      <div className="mx-auto max-w-7xl px-6 py-12">
        <div className="grid grid-cols-1 gap-12 md:grid-cols-5 mb-12">
          {/* Logo + tagline */}
          <div className="col-span-1 md:col-span-2 space-y-4">
            <div className="flex items-center gap-2">
              <SporeLogo size={28} />
              <span className="font-headline text-lg font-bold tracking-tight text-on-background">mycel</span>
            </div>
            <p className="text-sm text-on-surface-variant max-w-xs leading-relaxed">
              Your team of AI agents &mdash; working in your repositories,
              reachable in your apps, visible in one place. Open source.
            </p>
            <p className="text-xs text-on-surface-variant/60">
              &copy; {new Date().getFullYear()} mycel
            </p>
          </div>

          {/* Product */}
          <div className="space-y-4">
            <h2 className="text-xs font-label font-bold uppercase tracking-widest text-on-surface-variant/50">
              Product
            </h2>
            <nav
              aria-label="Product links"
              className="flex flex-col gap-2.5 text-sm text-on-surface-variant"
            >
              <Link
                href="/"
                className="hover:text-on-surface transition-colors"
              >
                Home
              </Link>
              <Link
                href="/#product"
                className="hover:text-on-surface transition-colors"
              >
                Features
              </Link>
              <Link
                href="/method"
                className="hover:text-on-surface transition-colors"
              >
                Method
              </Link>
              <Link
                href="/#install"
                className="hover:text-on-surface transition-colors"
              >
                Install
              </Link>
            </nav>
          </div>

          {/* Resources */}
          <div className="space-y-4">
            <h2 className="text-xs font-label font-bold uppercase tracking-widest text-on-surface-variant/50">
              Resources
            </h2>
            <nav
              aria-label="Resources links"
              className="flex flex-col gap-2.5 text-sm text-on-surface-variant"
            >
              <Link
                href="/docs"
                className="hover:text-on-surface transition-colors"
              >
                Docs
              </Link>
              <Link
                href="/docs#installation"
                className="hover:text-on-surface transition-colors"
              >
                Getting Started
              </Link>
              <Link
                href="https://github.com/rpuneet/mycel"
                className="hover:text-on-surface transition-colors"
                target="_blank"
                rel="noopener noreferrer"
              >
                GitHub
              </Link>
            </nav>
          </div>

          {/* Company */}
          <div className="space-y-4">
            <h2 className="text-xs font-label font-bold uppercase tracking-widest text-on-surface-variant/50">
              Company
            </h2>
            <nav
              aria-label="Company links"
              className="flex flex-col gap-2.5 text-sm text-on-surface-variant"
            >
              <Link
                href="mailto:puneet@mycel.dev"
                className="hover:text-on-surface transition-colors"
              >
                Contact
              </Link>
              <Link
                href="/privacy"
                className="hover:text-on-surface transition-colors"
              >
                Privacy
              </Link>
              <Link
                href="/terms"
                className="hover:text-on-surface transition-colors"
              >
                Terms
              </Link>
              <Link
                href="https://twitter.com/mycel_dev"
                className="hover:text-on-surface transition-colors"
                target="_blank"
                rel="noopener noreferrer"
              >
                Twitter @mycel_dev
              </Link>
            </nav>
          </div>
        </div>

        {/* Bottom bar — no border, use surface shift */}
        <div className="flex flex-col md:flex-row items-center justify-between gap-4 pt-8 bg-surface-container-lowest text-xs text-on-surface-variant/40">
          <p>&copy; {new Date().getFullYear()} mycel. All rights reserved.</p>
          <div className="flex items-center gap-6">
            <Link
              href="/privacy"
              className="hover:text-on-surface transition-colors"
            >
              Privacy Policy
            </Link>
            <Link
              href="/terms"
              className="hover:text-on-surface transition-colors"
            >
              Terms of Service
            </Link>
          </div>
        </div>
      </div>
    </footer>
  );
}
