import "./globals.css";
import { Fraunces, Inter, Space_Mono } from "next/font/google";
import { ThemeProvider } from "./_contexts/ThemeContext";
import { WebVitals } from "./_components/WebVitals";
import {
  OrganizationSchema,
  WebsiteSchema,
  ProductSchema,
} from "./_components/StructuredData";
import { SITE_URL, absoluteUrl } from "../lib/site";

/* ── Brand type: Fraunces display serif + Inter body + Space Mono labels.
 *    Self-hosted through next/font — no CSS @import, no layout shift. ── */
const fraunces = Fraunces({
  subsets: ["latin"],
  style: ["normal", "italic"],
  axes: ["opsz", "SOFT", "WONK"],
  variable: "--font-fraunces",
  display: "swap",
});

const inter = Inter({
  subsets: ["latin"],
  variable: "--font-inter",
  display: "swap",
});

const spaceMono = Space_Mono({
  subsets: ["latin"],
  weight: ["400", "700"],
  variable: "--font-space-mono",
  display: "swap",
});

export const viewport = {
  width: "device-width",
  initialScale: 1,
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#FAF5EC" },
    { media: "(prefers-color-scheme: dark)", color: "#17110C" },
  ],
};

export const metadata = {
  title: "mycel — your team of AI agents, run from one place",
  description:
    "mycel runs your team of AI agents from one place. They write code in your repositories, reach you on Slack, WhatsApp, and 20+ apps, and everything they do — every action, change, and dollar — stays on screen.",
  keywords:
    "AI agents, AI team, agent orchestration, Claude Code, multi-agent development, AI coding, cost tracking, software development",
  metadataBase: new URL(SITE_URL),
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
    url: SITE_URL,
    title: "mycel — your team of AI agents, run from one place",
    description:
      "Your agents write code in your repositories, reach you on Slack, WhatsApp, and 20+ apps, and everything they do — every action, change, and dollar — stays on screen.",
    siteName: "mycel",
    images: [
      {
        url: absoluteUrl("/og-image.png"),
        width: 1200,
        height: 630,
        alt: "mycel — your team of AI agents, run from one place",
      },
    ],
  },
  twitter: {
    card: "summary_large_image",
    title: "mycel — your team of AI agents, run from one place",
    description:
      "Your agents write code in your repositories, reach you on Slack, WhatsApp, and 20+ apps, and everything they do — every action, change, and dollar — stays on screen.",
    images: [absoluteUrl("/og-image.png")],
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
    <html
      lang="en"
      suppressHydrationWarning
      className={`${fraunces.variable} ${inter.variable} ${spaceMono.variable}`}
    >
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
        <script dangerouslySetInnerHTML={{ __html: `try{var t=localStorage.getItem('mycel-theme')||'dark';if(t==='dark'||(t==='system'&&window.matchMedia('(prefers-color-scheme:dark)').matches)){document.documentElement.classList.add('dark')}}catch(e){}` }} />
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
