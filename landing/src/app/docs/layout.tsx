import { Metadata } from "next";
import { BreadcrumbSchema } from "../_components/StructuredData";
import { SITE_URL, absoluteUrl } from "../../lib/site";

export const metadata: Metadata = {
  title: "Documentation — mycel | Quick Start, CLI Reference & Guides",
  description:
    "Complete mycel documentation: installation, quick start, agents and runtimes, channels and notifications, cost tracking, the full CLI reference, configuration, and MYCEL_* environment variables. CLI-first multi-agent orchestration.",
  alternates: {
    canonical: "/docs",
  },
  openGraph: {
    title: "mycel Documentation — Quick Start, CLI Reference & Guides",
    description:
      "Complete mycel documentation: installation, quick start, agents and runtimes, channels and notifications, cost tracking, the full CLI reference, and configuration.",
    url: absoluteUrl("/docs"),
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
    title: "mycel Documentation — Quick Start, CLI Reference & Guides",
    description:
      "Complete mycel documentation: installation, quick start, agents and runtimes, channels and notifications, cost tracking, the full CLI reference, and configuration.",
    images: [absoluteUrl("/og-image.png")],
    creator: "@mycel_dev",
  },
};

export default function DocsLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <>
      {BreadcrumbSchema([
        { name: "Home", url: SITE_URL },
        { name: "Documentation", url: absoluteUrl("/docs") },
      ])}
      {children}
    </>
  );
}
