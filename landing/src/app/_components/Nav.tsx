"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { motion, AnimatePresence } from "framer-motion";
import { SporeLogo } from "./SporeLogo";
import { useState, useRef, useEffect } from "react";
import { Menu, X, Copy, Check, Apple, Monitor, Container, Download } from "lucide-react";
import { ThemeToggle } from "./ThemeToggle";

// The logo is Home and the page IS the product, so no Home/Product tabs.
const links = [
  { href: "/docs", label: "Docs" },
  { href: "https://github.com/rpuneet/mycel", label: "GitHub" },
];

function Logo({ reserved = false }: { reserved?: boolean }) {
  return (
    <div className="flex items-center gap-2">
      {reserved ? (
        // On the home page the hero mark travels up and docks into this slot
        // (see HeroLogo); reserve its box so the wordmark doesn't shift.
        <span
          aria-hidden="true"
          className="block"
          style={{ width: 26, height: 26 }}
        />
      ) : (
        <SporeLogo size={28} />
      )}
      <span className="font-headline text-lg font-bold tracking-tight text-on-background">mycel</span>
    </div>
  );
}

function InstallRow({
  icon: Icon,
  label,
  cmd,
  copied,
  onCopy,
}: {
  icon: React.ComponentType<{ className?: string; "aria-hidden"?: boolean }>;
  label: string;
  cmd: string;
  copied: boolean;
  onCopy: () => void;
}) {
  const [hovered, setHovered] = useState(false);

  return (
    <div
      className="px-3 py-2.5 hover:bg-surface-container-high/30 transition-colors cursor-default"
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      <div className="flex items-center gap-2.5">
        <Icon
          className="h-4 w-4 text-on-surface-variant shrink-0"
          aria-hidden={true}
        />
        <span className="text-sm font-medium text-on-background">{label}</span>
      </div>
      <motion.div
        initial={false}
        animate={{
          height: hovered ? "auto" : 0,
          opacity: hovered ? 1 : 0,
          marginTop: hovered ? 8 : 0,
        }}
        transition={{ duration: 0.2, ease: "easeInOut" }}
        className="overflow-hidden"
      >
        <div className="flex items-center gap-1.5 bg-surface-container-highest/50 rounded px-2 py-1.5">
          <code className="text-xs font-label text-on-background flex-1 min-w-0 truncate">
            {cmd}
          </code>
          <button
            onClick={(e) => {
              e.stopPropagation();
              onCopy();
            }}
            className="shrink-0 p-1 rounded hover:bg-surface-container-high transition-colors"
            aria-label={`Copy ${label} install command`}
          >
            {copied ? (
              <Check className="h-3.5 w-3.5 text-green-500" />
            ) : (
              <Copy className="h-3.5 w-3.5 text-on-surface-variant" />
            )}
          </button>
        </div>
      </motion.div>
    </div>
  );
}

function GetStartedDropdown() {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  const platforms = [
    {
      icon: Apple,
      label: "macOS / Linux",
      cmd: "curl -fsSL https://raw.githubusercontent.com/rpuneet/mycel/main/scripts/install.sh | bash",
    },
    {
      icon: Monitor,
      label: "Homebrew",
      cmd: "brew install rpuneet/mycel/mycel",
    },
    {
      icon: Container,
      label: "Docker",
      cmd: "docker run -p 9374:9374 -v $(pwd):/workspace ghcr.io/rpuneet/mycel mycel up --addr 0.0.0.0:9374",
    },
  ];

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node))
        setOpen(false);
    }
    if (open) document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [open]);

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(!open)}
        className="inline-flex items-center gap-1.5 rounded-sm bg-primary px-4 py-1.5 text-[13px] font-medium text-primary-foreground transition-all hover:opacity-90 active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-primary cta-glow whitespace-nowrap"
      >
        Get Started
      </button>
      <AnimatePresence>
        {open && (
          <motion.div
            initial={{ opacity: 0, y: -4, scale: 0.95 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: -4, scale: 0.95 }}
            transition={{ duration: 0.15 }}
            className="absolute right-0 top-full mt-2 w-80 rounded-lg border border-outline-variant/20 bg-surface-container shadow-xl overflow-hidden z-50"
          >
            <div className="px-3 py-2 border-b border-outline-variant/20">
              <span className="text-[10px] font-semibold uppercase tracking-[0.15em] text-on-surface-variant">
                Install
              </span>
            </div>
            {platforms.map((p) => (
              <InstallRow
                key={p.label}
                icon={p.icon}
                label={p.label}
                cmd={p.cmd}
                copied={copied === p.label}
                onCopy={() => {
                  navigator.clipboard.writeText(p.cmd);
                  setCopied(p.label);
                  setTimeout(() => setCopied(null), 2000);
                }}
              />
            ))}
            <div className="px-3 py-2 border-t border-outline-variant/20 bg-surface-container-highest/30">
              <Link
                href="/docs#installation"
                onClick={() => setOpen(false)}
                className="text-[11px] text-on-surface-variant hover:text-on-surface transition-colors"
              >
                Full installation guide →
              </Link>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

export function Nav() {
  const [isOpen, setIsOpen] = useState(false);
  const [scrolled, setScrolled] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const pathname = usePathname();

  useEffect(() => {
    function onScroll() {
      setScrolled(window.scrollY > 10);
    }
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    }
    if (isOpen) document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [isOpen]);

  useEffect(() => {
    function handleEscape(event: KeyboardEvent) {
      if (event.key === "Escape") setIsOpen(false);
    }
    if (isOpen) document.addEventListener("keydown", handleEscape);
    return () => document.removeEventListener("keydown", handleEscape);
  }, [isOpen]);

  const isActive = (href: string) => {
    if (href === "/") return pathname === "/";
    return pathname.startsWith(href);
  };

  const handleLinkClick = () => setIsOpen(false);

  return (
    <header
      className={`fixed top-0 left-0 right-0 z-50 transition-all duration-300 ${
        scrolled
          ? "bg-[var(--nav-bg)] backdrop-blur-xl shadow-[0_1px_0_var(--glass-border)]"
          : "bg-transparent"
      }`}
    >
      <div
        className={`mx-auto flex max-w-6xl items-center justify-between px-4 transition-all duration-300 sm:px-6 ${
          scrolled ? "py-2" : "py-3.5"
        }`}
      >
        {/* Left: Logo */}
        <Link
          href="/"
          className="focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          aria-label="mycel home page"
        >
          <Logo reserved={pathname === "/"} />
        </Link>

        {/* Center: Nav links */}
        <nav
          aria-label="Main navigation"
          className="hidden md:flex items-center gap-1"
        >
          {links.map((l) => (
            <Link
              key={l.href}
              href={l.href}
              className={`relative px-3 py-1.5 text-[13px] font-medium transition-colors ${
                isActive(l.href)
                  ? "text-primary"
                  : "text-on-surface-variant hover:text-on-surface"
              }`}
            >
              {l.label}
              {isActive(l.href) && (
                <motion.div
                  layoutId="nav-underline"
                  className="absolute bottom-0 left-3 right-3 h-[2px] bg-primary"
                  transition={{ type: "spring", stiffness: 380, damping: 30 }}
                />
              )}
            </Link>
          ))}
        </nav>

        {/* Right: Theme toggle + Get Started dropdown */}
        <div className="hidden md:flex items-center gap-3">
          <ThemeToggle />
          <GetStartedDropdown />
        </div>

        {/* Mobile: persistent install CTA (appears once scrolled) + hamburger */}
        <div className="md:hidden flex items-center gap-2">
          <AnimatePresence>
            {scrolled && (
              <motion.div
                initial={{ opacity: 0, scale: 0.9 }}
                animate={{ opacity: 1, scale: 1 }}
                exit={{ opacity: 0, scale: 0.9 }}
                transition={{ duration: 0.18 }}
              >
                <Link
                  href="/#install"
                  className="inline-flex items-center gap-1.5 rounded-sm bg-primary px-3 py-1.5 text-[12px] font-semibold text-primary-foreground transition-all active:scale-[0.98]"
                >
                  <Download className="h-3.5 w-3.5" aria-hidden="true" />
                  Install
                </Link>
              </motion.div>
            )}
          </AnimatePresence>
          <ThemeToggle />
          <button
            onClick={() => setIsOpen(!isOpen)}
            className="p-1.5 hover:bg-surface-container-high/50 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            aria-label={isOpen ? "Close menu" : "Open menu"}
            aria-expanded={isOpen}
            aria-controls="mobile-menu"
          >
            <motion.div
              animate={isOpen ? "open" : "closed"}
              variants={{
                open: { rotate: 90 },
                closed: { rotate: 0 },
              }}
              transition={{ duration: 0.2 }}
            >
              {isOpen ? (
                <X size={20} aria-hidden="true" />
              ) : (
                <Menu size={20} aria-hidden="true" />
              )}
            </motion.div>
          </button>
        </div>
      </div>

      {/* Mobile Menu Panel */}
      <AnimatePresence>
        {isOpen && (
          <motion.div
            ref={menuRef}
            id="mobile-menu"
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.2, ease: "easeInOut" }}
            className="md:hidden bg-surface-container-low/95 backdrop-blur-xl"
          >
            <nav
              aria-label="Mobile navigation"
              className="flex flex-col px-4 py-3 space-y-1"
            >
              {links.map((l) => (
                <Link
                  key={l.href}
                  href={l.href}
                  onClick={handleLinkClick}
                  className={`px-3 py-2.5 text-sm font-medium transition-colors ${
                    isActive(l.href)
                      ? "text-primary bg-surface-container/50"
                      : "text-on-surface-variant hover:text-on-surface hover:bg-surface-container/30"
                  }`}
                >
                  {l.label}
                </Link>
              ))}
              <div className="h-px bg-outline-variant/20 my-1" />
              <div className="px-3 py-2">
                <div className="text-[10px] font-semibold uppercase tracking-[0.15em] text-on-surface-variant mb-2">
                  Install
                </div>
                <code className="block text-xs font-label text-on-background bg-surface-container-highest/50 rounded px-2.5 py-2 mb-1.5">
                  curl -fsSL https://raw.githubusercontent.com/rpuneet/mycel/main/scripts/install.sh | bash
                </code>
                <code className="block text-xs font-label text-on-background bg-surface-container-highest/50 rounded px-2.5 py-2 mb-1.5">
                  brew install rpuneet/mycel/mycel
                </code>
                <Link
                  href="/docs#installation"
                  onClick={handleLinkClick}
                  className="text-[11px] text-on-surface-variant hover:text-on-surface transition-colors"
                >
                  Full installation guide →
                </Link>
              </div>
              <div className="h-px bg-outline-variant/20 my-1" />
              <div className="px-3 py-2 flex items-center justify-between">
                <span className="text-sm font-medium text-on-surface-variant">
                  Theme
                </span>
                <ThemeToggle />
              </div>
            </nav>
          </motion.div>
        )}
      </AnimatePresence>
    </header>
  );
}
