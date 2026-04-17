"use client";

import { useState } from "react";
import Link from "next/link";
import { motion, AnimatePresence } from "framer-motion";
import {
  FadeUp,
  RevealSection,
  StaggerChildren,
  StaggerItem,
} from "../_components/Motion";

const CHECK = (
  <svg
    width="16"
    height="16"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2.5"
    strokeLinecap="round"
    strokeLinejoin="round"
    className="text-primary shrink-0"
  >
    <polyline points="20 6 9 17 4 12" />
  </svg>
);

const FAQS = [
  {
    question: "Is the local version truly free?",
    answer:
      "Yes. mycel is MIT-licensed open source. The local version includes all features — unlimited agents, channels, cost tracking, MCP integration. You only pay for the AI tokens you use with your own API keys.",
  },
  {
    question: "What is included in the Cloud waitlist?",
    answer:
      "Cloud gives you remote access to your mycel workspace via SSH, managed agent hosting, and priority support. Join the waitlist to be notified when it launches.",
  },
  {
    question: "Do you offer discounts for startups?",
    answer:
      "Yes. Contact us at hello@mycel.dev with details about your startup and we'll work something out.",
  },
  {
    question: "When will Enterprise be available?",
    answer:
      "Enterprise features are in development. Contact hello@mycel.dev to discuss your requirements and timeline.",
  },
];

function FAQAccordion() {
  const [openIndex, setOpenIndex] = useState<number | null>(null);

  return (
    <div>
      {FAQS.map((faq, i) => (
        <div
          key={i}
          className="border-b border-outline-variant/10 bg-surface-container"
        >
          <button
            onClick={() => setOpenIndex(openIndex === i ? null : i)}
            className="flex w-full items-center justify-between py-4 px-4 text-left"
            aria-expanded={openIndex === i}
          >
            <span className="font-headline text-sm font-semibold text-on-background">
              {faq.question}
            </span>
            <svg
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              className={`shrink-0 text-on-surface-variant transition-transform duration-200 ${
                openIndex === i ? "rotate-180" : ""
              }`}
            >
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </button>
          <AnimatePresence>
            {openIndex === i && (
              <motion.div
                initial={{ height: 0, opacity: 0 }}
                animate={{ height: "auto", opacity: 1 }}
                exit={{ height: 0, opacity: 0 }}
                transition={{ duration: 0.2 }}
                className="overflow-hidden"
              >
                <p className="px-4 pb-4 text-sm leading-relaxed text-on-surface-variant">
                  {faq.answer}
                </p>
              </motion.div>
            )}
          </AnimatePresence>
        </div>
      ))}
    </div>
  );
}

export default function PricingPage() {
  return (
    <main className="min-h-screen bg-background">
      {/* Hero */}
      <section className="hero-glow py-24">
        <div className="mx-auto max-w-5xl px-4 text-center">
          <FadeUp>
            <h1 className="font-headline text-5xl font-bold tracking-tight text-on-background lg:text-6xl">
              Scale your
              <br />
              <span className="accent-instrument text-primary">
                underground network.
              </span>
            </h1>
          </FadeUp>
          <FadeUp delay={0.1}>
            <p className="mx-auto mt-6 max-w-xl text-lg text-on-surface-variant">
              Simple, transparent pricing. Run it locally for free, or scale to
              the cloud when you&apos;re ready to cook.
            </p>
          </FadeUp>
        </div>
      </section>

      {/* Pricing Cards */}
      <section className="py-16">
        <StaggerChildren
          className="mx-auto grid max-w-5xl grid-cols-1 gap-6 px-4 lg:grid-cols-3"
          stagger={0.12}
        >
          {/* Free */}
          <StaggerItem>
            <div className="flex h-full flex-col rounded-sm bg-surface-container p-8">
              <span className="font-label text-sm text-on-surface-variant">
                01_
              </span>
              <h2 className="mt-2 font-headline text-2xl font-bold text-on-background">
                Free
              </h2>
              <div className="mt-4 flex items-baseline gap-2">
                <span className="font-headline text-4xl font-bold text-on-background">
                  $0
                </span>
                <span className="text-on-surface-variant">/ forever</span>
              </div>
              <p className="mt-3 text-sm text-on-surface-variant">
                For individuals and open-source projects running locally.
              </p>
              <ul className="mt-6 flex-1 space-y-3">
                {[
                  "Open source core",
                  "Local execution",
                  "Community support",
                  "Unlimited agents",
                ].map((f) => (
                  <li key={f} className="flex items-center gap-2 text-sm text-on-surface-variant">
                    {CHECK}
                    {f}
                  </li>
                ))}
              </ul>
              <Link
                href="/docs"
                className="mt-8 block w-full rounded-sm bg-primary py-3 text-center font-label text-sm font-semibold text-on-background transition-opacity hover:opacity-90"
              >
                Get Started
              </Link>
            </div>
          </StaggerItem>

          {/* Cloud */}
          <StaggerItem>
            <div className="relative flex h-full flex-col rounded-sm bg-surface-container-high p-8">
              <span className="absolute -top-3 left-1/2 -translate-x-1/2 rounded-sm bg-primary px-3 py-1 font-label text-xs font-semibold text-on-background">
                Most Popular
              </span>
              <span className="font-label text-sm text-on-surface-variant">
                02_
              </span>
              <h2 className="mt-2 font-headline text-2xl font-bold text-on-background">
                Cloud
              </h2>
              <div className="mt-4 flex items-baseline gap-2">
                <span className="font-headline text-4xl font-bold text-on-background">
                  &#8377;1,000
                </span>
                <span className="text-on-surface-variant">/ month</span>
              </div>
              <p className="mt-3 text-sm text-on-surface-variant">
                For small teams scaling their infrastructure.
              </p>
              <ul className="mt-6 flex-1 space-y-3">
                {[
                  "Everything in Free",
                  "Remote agents",
                  "Secure SSH access",
                  "Priority email support",
                  "Advanced telemetry",
                ].map((f) => (
                  <li key={f} className="flex items-center gap-2 text-sm text-on-surface-variant">
                    {CHECK}
                    {f}
                  </li>
                ))}
              </ul>
              <Link
                href="/waitlist"
                className="mt-8 block w-full rounded-sm border border-outline-variant/20 py-3 text-center font-label text-sm font-semibold text-on-background transition-colors hover:bg-surface-container"
              >
                Join Waitlist
              </Link>
            </div>
          </StaggerItem>

          {/* Enterprise */}
          <StaggerItem>
            <div className="flex h-full flex-col rounded-sm bg-surface-container p-8">
              <span className="font-label text-sm text-on-surface-variant">
                03_
              </span>
              <h2 className="mt-2 font-headline text-2xl font-bold text-on-background">
                Enterprise
              </h2>
              <div className="mt-4">
                <span className="font-headline text-4xl font-bold text-on-background">
                  Custom
                </span>
              </div>
              <p className="mt-3 text-sm text-on-surface-variant">
                For large orgs needing robust security and compliance.
              </p>
              <ul className="mt-6 flex-1 space-y-3">
                {[
                  "SSO Integration",
                  "Detailed audit logs",
                  "Dedicated support SLA",
                  "Custom agent limits",
                ].map((f) => (
                  <li key={f} className="flex items-center gap-2 text-sm text-on-surface-variant">
                    {CHECK}
                    {f}
                  </li>
                ))}
              </ul>
              <a
                href="mailto:hello@mycel.dev"
                className="mt-8 block text-center font-label text-sm text-primary transition-opacity hover:opacity-80"
              >
                Contact hello@mycel.dev
              </a>
            </div>
          </StaggerItem>
        </StaggerChildren>
      </section>

      {/* FAQ */}
      <section className="py-24">
        <div className="mx-auto max-w-3xl px-4">
          <RevealSection className="mb-10 text-center">
            <h2 className="font-headline text-3xl font-bold text-on-background">
              Frequently Asked Questions
            </h2>
          </RevealSection>
          <RevealSection delay={0.1}>
            <FAQAccordion />
          </RevealSection>
        </div>
      </section>
    </main>
  );
}
