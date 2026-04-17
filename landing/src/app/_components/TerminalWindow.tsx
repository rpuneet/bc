"use client";

import { motion } from "framer-motion";

interface TerminalWindowProps {
  title?: string;
  children: React.ReactNode;
  className?: string;
  animate?: boolean;
}

export function TerminalWindow({
  title = "bash",
  children,
  className = "",
  animate = true,
}: TerminalWindowProps) {
  const Wrapper = animate ? motion.div : "div";
  const wrapperProps = animate
    ? {
        initial: { opacity: 0, y: 20 },
        whileInView: { opacity: 1, y: 0 },
        viewport: { once: true, margin: "-60px" },
        transition: { duration: 0.5, ease: "easeOut" },
      }
    : {};

  return (
    <Wrapper
      {...(wrapperProps as Record<string, unknown>)}
      className={`glass-panel terminal-glow rounded-lg overflow-hidden ${className}`}
    >
      {/* Title bar */}
      <div className="flex items-center gap-2 px-4 py-2.5 bg-surface-container-low/80">
        {/* Traffic light dots — subtle, monochrome */}
        <div className="flex items-center gap-1.5">
          <div className="w-2.5 h-2.5 rounded-full bg-on-surface-variant/20" />
          <div className="w-2.5 h-2.5 rounded-full bg-on-surface-variant/15" />
          <div className="w-2.5 h-2.5 rounded-full bg-on-surface-variant/10" />
        </div>
        {title && (
          <span className="text-[11px] font-mono text-on-surface-variant/50 ml-2">
            {title}
          </span>
        )}
      </div>

      {/* Content */}
      <div className="p-4 font-mono text-sm text-terminal-text bg-terminal-bg">
        {children}
      </div>
    </Wrapper>
  );
}
