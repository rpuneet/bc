"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { motion, AnimatePresence } from "framer-motion";
import { useState, useRef, useEffect } from "react";
import { Menu, X } from "lucide-react";
import { ThemeToggle } from "./ThemeToggle";

const links = [
  { href: "/", label: "Home" },
  { href: "/product", label: "Product" },
  { href: "/method", label: "Method" },
  { href: "/docs", label: "Docs" },
  { href: "/pricing", label: "Pricing" },
];

function Logo() {
  return (
    <div className="flex items-center">
      <span className="font-mono text-lg text-primary">&gt;</span>
      <span className="font-mono text-lg font-bold tracking-tight text-on-surface ml-1">
        mycel
      </span>
    </div>
  );
}

function NavLink({
  href,
  label,
  active,
  onClick,
}: {
  href: string;
  label: string;
  active: boolean;
  onClick?: () => void;
}) {
  return (
    <Link
      href={href}
      onClick={onClick}
      className={`relative px-3 py-1.5 text-[13px] font-medium transition-colors ${
        active
          ? "text-primary"
          : "text-on-surface-variant hover:text-on-surface"
      }`}
    >
      {label}
      {active && (
        <motion.div
          layoutId="nav-underline"
          className="absolute bottom-0 left-3 right-3 h-[2px] bg-primary"
          transition={{ type: "spring", stiffness: 380, damping: 30 }}
        />
      )}
    </Link>
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

  return (
    <header
      className={`fixed top-0 left-0 right-0 z-50 transition-all duration-300 ${
        scrolled
          ? "bg-[var(--nav-bg)] backdrop-blur-xl shadow-[0_1px_0_var(--glass-border)]"
          : "bg-transparent"
      }`}
    >
      <div className="mx-auto flex max-w-6xl items-center justify-between px-4 py-3 sm:px-6">
        {/* Left: Logo */}
        <Link
          href="/"
          className="focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          aria-label="mycel home page"
        >
          <Logo />
        </Link>

        {/* Center: Nav links */}
        <nav
          aria-label="Main navigation"
          className="hidden md:flex items-center gap-1"
        >
          {links.map((l) => (
            <NavLink
              key={l.href}
              href={l.href}
              label={l.label}
              active={isActive(l.href)}
            />
          ))}
        </nav>

        {/* Right: Theme toggle + Get Started */}
        <div className="hidden md:flex items-center gap-3">
          <ThemeToggle />
          <Link
            href="/docs#installation"
            className="inline-flex items-center rounded-sm bg-primary px-4 py-1.5 text-[13px] font-medium text-primary-foreground transition-all hover:opacity-90 active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-primary cta-glow"
          >
            Get Started
          </Link>
        </div>

        {/* Mobile: hamburger */}
        <div className="md:hidden flex items-center gap-2">
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
                  onClick={() => setIsOpen(false)}
                  className={`px-3 py-2.5 text-sm font-medium transition-colors ${
                    isActive(l.href)
                      ? "text-primary bg-surface-container/50"
                      : "text-on-surface-variant hover:text-on-surface hover:bg-surface-container/30"
                  }`}
                >
                  {l.label}
                </Link>
              ))}
              <div className="pt-2">
                <Link
                  href="/docs#installation"
                  onClick={() => setIsOpen(false)}
                  className="block w-full text-center rounded-sm bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground"
                >
                  Get Started
                </Link>
              </div>
            </nav>
          </motion.div>
        )}
      </AnimatePresence>
    </header>
  );
}
