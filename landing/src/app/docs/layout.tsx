import { Metadata } from "next";
import { BreadcrumbSchema } from "../_components/StructuredData";

export const metadata: Metadata = {
  title: "Documentation — mycel | Quick Start, CLI Reference & Guides",
  description:
    "Complete mycel documentation: installation, quick start, all 55 CLI commands, configuration, presets, and environment variables. CLI-first multi-agent orchestration.",
  alternates: {
    canonical: "/docs",
  },
  openGraph: {
    title: "mycel Documentation — Quick Start, CLI Reference & Guides",
    description:
      "Complete mycel documentation: installation, quick start, all 55 CLI commands, configuration, presets, and environment variables.",
    url: "https://mycel.dev/docs",
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
    title: "mycel Documentation — Quick Start, CLI Reference & Guides",
    description:
      "Complete mycel documentation: installation, quick start, all 55 CLI commands, configuration, presets, and environment variables.",
    images: ["https://mycel.dev/og-image.png"],
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
        { name: "Home", url: "https://mycel.dev" },
        { name: "Documentation", url: "https://mycel.dev/docs" },
      ])}
      {children}
    </>
  );
}
