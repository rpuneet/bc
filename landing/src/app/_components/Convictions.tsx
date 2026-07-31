import Link from "next/link";
import { FadeUp, ScrollReveal } from "./Motion";

/**
 * The six convictions, folded into the home page as a dense reference grid
 * — each principle stated once, then pointed at the surface where it's
 * visible in the product. No essay, no repetition; mention it and show
 * where it lives.
 */
const CONVICTIONS: {
  name: string;
  line: string;
  inAction: string;
  href: string;
}[] = [
  {
    name: "Isolation",
    line: "Every agent works in its own git worktree — parallel writers never collide.",
    inAction: "Code view",
    href: "/#product",
  },
  {
    name: "Communication",
    line: "Agents live in the channels you already use, addressed by name.",
    inAction: "Apps",
    href: "/#product",
  },
  {
    name: "Visibility",
    line: "A face per agent, then every action expandable — no black box.",
    inAction: "Live",
    href: "/#product",
  },
  {
    name: "Cost",
    line: "Spend read from each agent's own records, per model, over time.",
    inAction: "Insights",
    href: "/#product",
  },
  {
    name: "Persistence",
    line: "A tool runs once; an agent keeps going until the job is finished.",
    inAction: "Live history",
    href: "/#product",
  },
  {
    name: "Openness",
    line: "Free, MIT-licensed, and running entirely on your machine.",
    inAction: "GitHub",
    href: "https://github.com/rpuneet/mycel",
  },
];

export function Convictions() {
  return (
    <section id="convictions" className="scroll-mt-24 py-14 sm:py-16">
      <div className="mx-auto max-w-5xl px-4 sm:px-6">
        <FadeUp>
          <span className="deck-eyebrow">What it&rsquo;s built on</span>
          <h2 className="mt-4 max-w-2xl font-headline text-3xl font-semibold tracking-tight text-on-background sm:text-4xl">
            Six convictions, each one shipping.
          </h2>
        </FadeUp>

        <div className="mt-9 grid gap-x-8 gap-y-6 sm:grid-cols-2 lg:grid-cols-3">
          {CONVICTIONS.map((c, i) => (
            <ScrollReveal key={c.name} delay={(i % 3) * 0.06}>
              <div className="border-t border-outline-variant/20 pt-4">
                <div className="flex items-baseline justify-between gap-3">
                  <h3 className="font-headline text-lg font-semibold tracking-tight text-on-background">
                    {c.name}
                  </h3>
                  <Link
                    href={c.href}
                    className="shrink-0 font-label text-[11px] font-semibold uppercase tracking-[0.1em] text-primary-text/70 transition-colors hover:text-primary"
                  >
                    {c.inAction} &rarr;
                  </Link>
                </div>
                <p className="mt-1.5 font-body text-[14px] leading-relaxed text-on-surface-variant">
                  {c.line}
                </p>
              </div>
            </ScrollReveal>
          ))}
        </div>
      </div>
    </section>
  );
}
