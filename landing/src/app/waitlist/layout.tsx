import { Metadata } from "next";
import { BreadcrumbSchema, FAQSchema } from "../_components/StructuredData";

export const metadata: Metadata = {
  title: "Early Access — mycel | Multi-Agent Orchestration for AI Coding Agents",
  description:
    "Get early access to mycel, the open-source CLI-first multi-agent orchestration tool. Run multiple AI coding agents simultaneously with zero conflicts.",
  alternates: {
    canonical: "/waitlist",
  },
  openGraph: {
    title: "mycel Early Access — Multi-Agent Orchestration",
    description:
      "Get early access to mycel, the open-source tool for orchestrating AI coding agents with zero conflicts.",
    url: "https://mycel.dev/waitlist",
    siteName: "mycel",
    type: "website",
    images: [
      {
        url: "https://mycel.dev/og-image.png",
        width: 1200,
        height: 630,
        alt: "mycel - Multi-Agent Orchestration Platform",
      },
    ],
  },
  twitter: {
    card: "summary_large_image",
    title: "mycel Early Access — Multi-Agent Orchestration",
    description:
      "Get early access to mycel, the open-source tool for orchestrating AI coding agents with zero conflicts.",
    images: ["https://mycel.dev/og-image.png"],
    creator: "@myceldev",
  },
};

export default function WaitlistLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <>
      {BreadcrumbSchema([
        { name: "Home", url: "https://mycel.dev" },
        { name: "Early Access", url: "https://mycel.dev/waitlist" },
      ])}
      {FAQSchema([
        {
          question: "What is mycel?",
          answer:
            "mycel is a CLI-first multi-agent orchestration tool. It coordinates multiple AI coding agents — like Claude Code, Cursor, Codex, Gemini, and others — so they can work in parallel on isolated git worktrees without merge conflicts or context loss.",
        },
        {
          question: "How is this different from using a single AI agent?",
          answer:
            "A single agent works on one task at a time. mycel runs multiple agents simultaneously, each on its own branch, communicating through structured channels — with cost controls and real-time visibility.",
        },
        {
          question: "Which AI tools does mycel support?",
          answer:
            "mycel is agent-agnostic. It works with Claude Code, Cursor, Codex, Gemini, Aider, OpenCode, OpenClaw, and any CLI-based coding assistant. You configure providers in a simple TOML file and mycel handles the coordination.",
        },
        {
          question: "Do I need to change how my agents work?",
          answer:
            "No. mycel orchestrates agents you already use with zero code changes. Your agents keep running the same commands — mycel adds the coordination layer on top: worktree isolation, channels for communication, persistent memory, and cost tracking.",
        },
        {
          question: "Is mycel open source?",
          answer:
            "Yes. mycel is open source and runs entirely on your local machine. You can inspect the code, contribute, and self-host. Early access gives you the full platform — CLI, Web UI dashboard, and all features.",
        },
        {
          question: "Does mycel require a cloud account?",
          answer:
            "No. mycel is local-first. It runs on your machine using tmux sessions and local git worktrees. No data leaves your machine unless you configure external AI providers (which you control).",
        },
        {
          question: "How does early access work?",
          answer:
            "Sign up with your email and we'll notify you when mycel is ready. You'll get installation instructions, full access to all features, and a direct line to the team for feedback and support.",
        },
      ])}
      {children}
    </>
  );
}
