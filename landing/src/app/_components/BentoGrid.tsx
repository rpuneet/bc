"use client";

import Image from "next/image";
import { motion, useInView } from "framer-motion";
import { useRef } from "react";
import {
  Users,
  MessageSquare,
  DollarSign,
  Shield,
  GitBranch,
  Clock,
  Lock,
  Plug,
  Activity,
  Stethoscope,
} from "lucide-react";

const fadeUp = {
  hidden: { opacity: 0, y: 20 },
  visible: { opacity: 1, y: 0, transition: { duration: 0.5, ease: "easeOut" as const } },
};

interface BentoCardProps {
  title: string;
  description: string;
  icon: React.ComponentType<{ className?: string }>;
  screenshot?: string;
  screenshotAlt?: string;
  className?: string;
  delay?: number;
}

function BentoCard({
  title,
  description,
  icon: Icon,
  screenshot,
  screenshotAlt,
  className = "",
  delay = 0,
}: BentoCardProps) {
  const ref = useRef<HTMLDivElement>(null);
  const inView = useInView(ref, { once: true, margin: "-50px" });

  return (
    <motion.div
      ref={ref}
      initial="hidden"
      animate={inView ? "visible" : "hidden"}
      variants={fadeUp}
      transition={{ delay }}
      className={`bento-card group rounded-xl border border-border bg-card/80 backdrop-blur-sm overflow-hidden ${className}`}
    >
      {screenshot && (
        <div className="overflow-hidden shadow-inner">
          <Image
            src={screenshot}
            alt={screenshotAlt || title}
            width={600}
            height={375}
            className="w-full h-auto transition-transform duration-300 group-hover:scale-[1.02]"
          />
        </div>
      )}
      <div className="p-5">
        <div className="flex items-center gap-2 mb-2">
          <Icon className="h-4 w-4 text-primary/70" />
          <h3 className="font-semibold text-sm tracking-tight">{title}</h3>
        </div>
        <p className="text-xs text-muted-foreground leading-relaxed">
          {description}
        </p>
      </div>
    </motion.div>
  );
}

export function BentoGrid() {
  return (
    <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-4">
      {/* Large cards - span 3 cols each */}
      <BentoCard
        title="Agents"
        description="Spawn agents with one command. Each gets its own worktree, role, and tool — visible in real time."
        icon={Users}
        screenshot="/screenshots/dashboard-02-agents.png"
        screenshotAlt="Agent management table showing names, roles, tools, and statuses"
        className="col-span-2 md:col-span-2 lg:col-span-3"
        delay={0}
      />
      <BentoCard
        title="Channels"
        description="Let agents coordinate through persistent channels. @mentions, handoffs, and full message history."
        icon={MessageSquare}
        screenshot="/screenshots/dashboard-03-channels.png"
        screenshotAlt="Channel view showing real-time agent-to-agent messages"
        className="col-span-2 md:col-span-2 lg:col-span-3"
        delay={0.1}
      />

      {/* Medium cards - span 2 cols each */}
      <BentoCard
        title="Costs"
        description="Track every dollar per agent. Set budgets that automatically pause agents before overspending."
        icon={DollarSign}
        screenshot="/screenshots/dashboard-04-costs.png"
        screenshotAlt="Cost tracking with daily trend chart and per-agent breakdown"
        className="col-span-2"
        delay={0.2}
      />
      <BentoCard
        title="Roles"
        description="Define what each agent can do. Engineers implement, managers assign, QA validates — or create your own."
        icon={Shield}
        screenshot="/screenshots/dashboard-05-roles.png"
        screenshotAlt="Role configuration cards with capability settings"
        className="col-span-2"
        delay={0.25}
      />
      <BentoCard
        title="Worktrees"
        description="Every agent gets its own git worktree. Parallel edits, zero merge conflicts."
        icon={GitBranch}
        className="col-span-2"
        delay={0.3}
      />

      {/* Small cards */}
      <BentoCard
        title="Cron"
        description="Automate recurring tasks on a schedule. Lint nightly, run tests hourly."
        icon={Clock}
        className="col-span-1"
        delay={0.35}
      />
      <BentoCard
        title="Secrets"
        description="Store API keys encrypted at rest. Injected into agents, never in plaintext."
        icon={Lock}
        className="col-span-1"
        delay={0.4}
      />
      <BentoCard
        title="MCP"
        description="Plug in any MCP server. Agents inherit tools on spawn."
        icon={Plug}
        className="col-span-1"
        delay={0.45}
      />
      <BentoCard
        title="Stats"
        description="See CPU, memory, disk, and agent activity in one view."
        icon={Activity}
        className="col-span-1"
        delay={0.5}
      />
      <BentoCard
        title="Doctor"
        description="One command to diagnose and auto-fix workspace problems."
        icon={Stethoscope}
        className="col-span-2 sm:col-span-1"
        delay={0.55}
      />
    </div>
  );
}
