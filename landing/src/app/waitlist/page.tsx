"use client";

import { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { FadeUp } from "../_components/Motion";
import { Nav } from "../_components/Nav";
import { Footer } from "../_components/Footer";

const GOOGLE_FORM_ACTION =
  "https://docs.google.com/forms/d/e/1FAIpQLSc_aJ3S3nV5EizpkzTZnN7H5UykoANpC8jet2M7J0Qo3rhG8Q/formResponse";
const GOOGLE_FORM_EMAIL_FIELD = "entry.843755864";

export default function WaitlistPage() {
  const [email, setEmail] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    const emailRegex = /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/;
    if (!emailRegex.test(email)) {
      setError("Please enter a valid email address.");
      return;
    }

    setSubmitting(true);

    try {
      await new Promise<void>((resolve, reject) => {
        const iframe = document.createElement("iframe");
        iframe.name = "mycel-waitlist-frame";
        iframe.style.display = "none";
        document.body.appendChild(iframe);

        const form = document.createElement("form");
        form.method = "POST";
        form.action = GOOGLE_FORM_ACTION;
        form.target = "mycel-waitlist-frame";

        const input = document.createElement("input");
        input.type = "hidden";
        input.name = GOOGLE_FORM_EMAIL_FIELD;
        input.value = email;
        form.appendChild(input);

        const cleanup = () => {
          try {
            document.body.removeChild(iframe);
          } catch {
            /* already removed */
          }
          try {
            document.body.removeChild(form);
          } catch {
            /* already removed */
          }
        };

        const timeout = setTimeout(() => {
          cleanup();
          resolve();
        }, 5000);

        iframe.onload = () => {
          clearTimeout(timeout);
          cleanup();
          resolve();
        };
        iframe.onerror = () => {
          clearTimeout(timeout);
          cleanup();
          reject(
            new Error(
              "Network error — please check your connection and try again.",
            ),
          );
        };

        document.body.appendChild(form);
        form.submit();
      });

      setSubmitted(true);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Something went wrong. Please try again.",
      );
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="flex min-h-screen flex-col bg-background">
      <Nav />
      <main className="hero-glow flex flex-1 items-center justify-center">
        <div className="mx-auto max-w-2xl px-4 text-center">
        <FadeUp delay={0.1}>
          <h1 className="font-headline text-4xl font-bold tracking-tight text-on-background lg:text-6xl">
            Orchestrate{" "}
            <span className="font-bold">AI agents</span>
            <br />
            <span className="font-headline">from one place.</span>
          </h1>
        </FadeUp>

        <FadeUp delay={0.2}>
          <p className="mx-auto mt-6 max-w-lg text-lg leading-relaxed text-on-surface-variant">
            mycel Cloud adds remote SSH access, managed agent hosting, and
            priority support to the full-featured open-source CLI.
          </p>
        </FadeUp>

        {/* Email form */}
        <FadeUp delay={0.3}>
          <div className="mx-auto mt-10 max-w-md">
            <AnimatePresence mode="wait">
              {!submitted ? (
                <motion.form
                  key="form"
                  exit={{ opacity: 0, scale: 0.95 }}
                  onSubmit={handleSubmit}
                  className="flex flex-col gap-3 sm:flex-row"
                >
                  <label htmlFor="waitlist-email" className="sr-only">
                    Email address
                  </label>
                  <input
                    id="waitlist-email"
                    name="email"
                    type="email"
                    required
                    autoComplete="email"
                    value={email}
                    onChange={(e) => {
                      setEmail(e.target.value);
                      setError(null);
                    }}
                    placeholder="enter_email_address..."
                    maxLength={254}
                    className="flex-1 rounded-sm bg-surface-container-high px-4 py-3 font-label text-sm text-on-background outline-none placeholder:text-on-surface-variant/40 focus:ring-1 focus:ring-primary/30"
                  />
                  <button
                    type="submit"
                    disabled={submitting}
                    className="rounded-sm bg-primary px-6 py-3 font-label text-sm font-semibold text-on-background transition-opacity hover:opacity-90 disabled:opacity-50"
                  >
                    {submitting ? "Joining..." : "Join Waitlist"}
                  </button>
                </motion.form>
              ) : (
                <motion.div
                  key="success"
                  initial={{ opacity: 0, scale: 0.9 }}
                  animate={{ opacity: 1, scale: 1 }}
                  className="rounded-sm bg-surface-container p-6"
                >
                  <p className="font-label text-sm text-primary">
                    You&apos;re on the list.
                  </p>
                  <p className="mt-2 text-sm text-on-surface-variant">
                    We&apos;ll notify you when mycel Cloud is ready.
                  </p>
                  <button
                    onClick={() => {
                      setSubmitted(false);
                      setEmail("");
                    }}
                    className="mt-3 text-xs font-label text-primary hover:underline"
                  >
                    use a different email
                  </button>
                </motion.div>
              )}
            </AnimatePresence>
            {error && (
              <p className="mt-2 text-center font-label text-xs text-red-400" role="alert">
                {error}
              </p>
            )}
          </div>
        </FadeUp>

        {/* Stats bar */}
        <FadeUp delay={0.4}>
          <p className="mt-12 font-label text-sm uppercase tracking-widest text-on-surface-variant">
            4+ PROVIDERS &bull; OPEN SOURCE &bull; LOCAL-FIRST
          </p>
        </FadeUp>
        </div>
      </main>
      <Footer />
    </div>
  );
}
