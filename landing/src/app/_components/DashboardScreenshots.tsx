"use client";

import { useState } from "react";
import Image from "next/image";
import { motion, AnimatePresence } from "framer-motion";

const TABS = [
  {
    id: "dashboard",
    label: "Dashboard",
    src: "/screenshots/dashboard-01-home.png",
    alt: "mycel dashboard showing active agents, channels, total cost, and token usage",
  },
  {
    id: "agents",
    label: "Agents",
    src: "/screenshots/dashboard-02-agents.png",
    alt: "mycel agents table showing agent names, roles, tools, tasks, and statuses",
  },
  {
    id: "channels",
    label: "Channels",
    src: "/screenshots/dashboard-03-channels.png",
    alt: "mycel channel view showing real-time agent-to-agent communication",
  },
  {
    id: "costs",
    label: "Costs",
    src: "/screenshots/dashboard-04-costs.png",
    alt: "mycel cost tracking with daily trend chart and per-agent cost breakdown",
  },
  {
    id: "stats",
    label: "Stats",
    src: "/screenshots/dashboard-10-stats-loaded.png",
    alt: "mycel stats overview showing system metrics and agent performance",
  },
] as const;

export function DashboardScreenshots() {
  const [active, setActive] = useState(0);

  return (
    <div className="flex flex-col gap-4">
      {/* Tab bar */}
      <div className="flex items-center gap-1 rounded-lg border border-border bg-card/80 backdrop-blur-sm p-1 self-center">
        {TABS.map((tab, i) => (
          <button
            key={tab.id}
            onClick={() => setActive(i)}
            className={`rounded-md px-4 py-2 text-sm font-medium transition-all ${
              i === active
                ? "bg-primary text-primary-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground hover:bg-accent/20"
            }`}
            aria-label={`View ${tab.label} screenshot`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Browser chrome + Screenshot */}
      <div className="overflow-hidden rounded-xl border border-border shadow-2xl bg-card">
        {/* Browser chrome header */}
        <div className="flex items-center gap-3 border-b border-border bg-muted/50 px-4 py-2.5">
          <div className="flex gap-1.5" aria-hidden="true">
            <span className="h-2.5 w-2.5 rounded-full bg-[var(--traffic-red)]" />
            <span className="h-2.5 w-2.5 rounded-full bg-[var(--traffic-yellow)]" />
            <span className="h-2.5 w-2.5 rounded-full bg-[var(--traffic-green)]" />
          </div>
          <div className="flex-1 rounded-md bg-background/60 border border-border px-3 py-1 text-xs text-muted-foreground font-mono">
            localhost:9374
          </div>
        </div>

        {/* Screenshot with crossfade */}
        <div className="relative">
          <AnimatePresence mode="wait">
            <motion.div
              key={TABS[active].id}
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.2 }}
            >
              <Image
                src={TABS[active].src}
                alt={TABS[active].alt}
                width={1200}
                height={750}
                className="w-full h-auto"
                priority={active === 0}
              />
            </motion.div>
          </AnimatePresence>
        </div>
      </div>
    </div>
  );
}
