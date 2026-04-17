import "./globals.css";
import { ThemeProvider } from "./_contexts/ThemeContext";
import { WebVitals } from "./_components/WebVitals";
import {
  OrganizationSchema,
  WebsiteSchema,
  ProductSchema,
} from "./_components/StructuredData";

export const viewport = {
  width: "device-width",
  initialScale: 1,
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#FBF7F2" },
    { media: "(prefers-color-scheme: dark)", color: "#0C0A08" },
  ],
};

export const metadata = {
  title: "mycel — The Underground Network for AI Agents",
  description:
    "Orchestrate AI agents from your terminal. Isolated worktrees, shared channels, cost controls.",
  keywords:
    "AI agents, agent orchestration, Claude Code, multi-agent development, git worktrees, persistent memory, cost-aware AI, software development",
  metadataBase: new URL("https://mycel.dev"),
  alternates: {
    canonical: "/",
  },
  robots: {
    index: true,
    follow: true,
    googleBot: {
      index: true,
      follow: true,
      "max-snippet": -1,
      "max-image-preview": "large",
      "max-video-preview": -1,
    },
  },
  openGraph: {
    type: "website",
    locale: "en_US",
    url: "https://mycel.dev",
    title: "mycel — The Underground Network for AI Agents",
    description:
      "Orchestrate AI agents from your terminal. Isolated worktrees, shared channels, cost controls.",
    siteName: "mycel",
    images: [
      {
        url: "https://mycel.dev/og-image.png",
        width: 1200,
        height: 630,
        alt: "mycel — The Underground Network for AI Agents",
      },
    ],
  },
  twitter: {
    card: "summary_large_image",
    title: "mycel — The Underground Network for AI Agents",
    description:
      "Orchestrate AI agents from your terminal. Isolated worktrees, shared channels, cost controls.",
    images: ["https://mycel.dev/og-image.png"],
    creator: "@mycel_dev",
  },
  authors: [
    {
      name: "mycel team",
      url: "https://github.com/rpuneet",
    },
  ],
  creator: "mycel team",
  publisher: "mycel",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <meta charSet="utf-8" />
        <meta httpEquiv="x-ua-compatible" content="ie=edge" />
        <link rel="icon" type="image/svg+xml" href="/favicon.svg" />
        <link
          rel="apple-touch-icon"
          sizes="180x180"
          href="/apple-touch-icon.png"
        />
        <link rel="dns-prefetch" href="https://github.com" />
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link
          rel="preconnect"
          href="https://fonts.gstatic.com"
          crossOrigin="anonymous"
        />
        {/* eslint-disable-next-line @next/next/no-page-custom-font */}
        <link
          href="https://fonts.googleapis.com/css2?family=Instrument+Serif:ital@0;1&family=Inter:wght@400;500&family=Space+Grotesk:wght@300;400;500;600;700&family=Space+Mono:wght@400;700&display=swap"
          rel="stylesheet"
        />
      </head>
      <body className="bg-background text-on-background font-body antialiased transition-colors duration-300">
        <WebVitals />
        <OrganizationSchema />
        <WebsiteSchema />
        <ProductSchema />
        <ThemeProvider>{children}</ThemeProvider>
      </body>
    </html>
  );
}
