import { Metadata } from "next";
import { BreadcrumbSchema } from "../_components/StructuredData";

export const metadata: Metadata = {
  title: "Product — mycel | Agent Orchestration, Channels, Costs & More",
  description:
    "Explore mycel's multi-agent orchestration platform: agent lifecycle management, inter-agent channels, cost controls, role-based permissions, cron jobs, and more. A CLI-first orchestration platform.",
  alternates: {
    canonical: "/product",
  },
  openGraph: {
    title: "mycel Product — Agent Orchestration, Channels, Costs & More",
    description:
      "Explore mycel's multi-agent orchestration platform: agent lifecycle, channels, cost controls, roles, and cron jobs. A CLI-first orchestration platform.",
    url: "https://mycel.dev/product",
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
    title: "mycel Product — Agent Orchestration, Channels, Costs & More",
    description:
      "Explore mycel's multi-agent orchestration platform: agent lifecycle, channels, cost controls, roles, and cron jobs. A CLI-first orchestration platform.",
    images: ["https://mycel.dev/og-image.png"],
    creator: "@myceldev",
  },
};

export default function ProductLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <>
      {BreadcrumbSchema([
        { name: "Home", url: "https://mycel.dev" },
        { name: "Product", url: "https://mycel.dev/product" },
      ])}
      {children}
    </>
  );
}
