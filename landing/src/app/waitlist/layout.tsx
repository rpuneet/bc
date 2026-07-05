import { Metadata } from "next";
import { absoluteUrl } from "../../lib/site";

export const metadata: Metadata = {
  title: "Join the Waitlist - mycel",
  description:
    "Orchestrate AI agents from your terminal. Join the waitlist for mycel Cloud — the local-first, provider-agnostic network layer for developers.",
  alternates: {
    canonical: "/waitlist",
  },
  openGraph: {
    title: "mycel Waitlist — Multi-Agent Orchestration",
    description:
      "Join the waitlist for mycel Cloud. Orchestrate AI agents from your terminal.",
    url: absoluteUrl("/waitlist"),
    siteName: "mycel",
    type: "website",
    images: [
      {
        url: absoluteUrl("/og-image.png"),
        width: 1200,
        height: 630,
        alt: "mycel - Multi-Agent Orchestration Platform",
      },
    ],
  },
  twitter: {
    card: "summary_large_image",
    title: "mycel Waitlist — Multi-Agent Orchestration",
    description:
      "Join the waitlist for mycel Cloud. Orchestrate AI agents from your terminal.",
    images: [absoluteUrl("/og-image.png")],
    creator: "@mycel_dev",
  },
};

export default function WaitlistLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return children;
}
