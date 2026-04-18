"use client";

interface Tool {
  name: string;
  url: string;
  logo: string;
}

const TOOLS: Tool[] = [
  {
    name: "Claude Code",
    url: "https://docs.anthropic.com/en/docs/claude-code",
    logo: "https://www.google.com/s2/favicons?domain=claude.ai&sz=128",
  },
  {
    name: "Cursor",
    url: "https://cursor.com",
    logo: "https://cursor.com/apple-touch-icon.png",
  },
  {
    name: "Codex",
    url: "https://openai.com/codex",
    logo: "https://www.google.com/s2/favicons?domain=openai.com&sz=128",
  },
  {
    name: "Gemini",
    url: "https://gemini.google.com",
    logo: "https://www.gstatic.com/lamda/images/gemini_sparkle_v002_d4735304ff6292a690345.svg",
  },
];

const Y_OFFSETS = [0, 14, -8, 18];

function ToolChip({ tool, index }: { tool: Tool; index: number }) {
  const isExternal = tool.url.startsWith("http");
  const yOffset = Y_OFFSETS[index % Y_OFFSETS.length];

  return (
    <a
      href={tool.url}
      target={isExternal ? "_blank" : undefined}
      rel={isExternal ? "noopener noreferrer" : undefined}
      className="group inline-flex items-center gap-3 shrink-0 mx-7 transition-opacity duration-200 opacity-70 hover:opacity-100"
      style={{ transform: `translateY(${yOffset}px)` }}
    >
      <div className="h-10 w-10 shrink-0 rounded-xl overflow-hidden">
        {tool.logo ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={tool.logo}
            alt=""
            width={40}
            height={40}
            className="h-10 w-10 object-contain"
            loading="lazy"
          />
        ) : (
          <div className="h-10 w-10 rounded-xl bg-border/40 flex items-center justify-center">
            <svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
              className="h-5 w-5 text-muted-foreground/60"
            >
              <path d="M12 5v14M5 12h14" />
            </svg>
          </div>
        )}
      </div>
      <span className="text-sm font-medium text-foreground/60 group-hover:text-foreground whitespace-nowrap transition-colors duration-200">
        {tool.name}
      </span>
    </a>
  );
}

export function ToolMarquee() {
  return (
    <div className="relative overflow-hidden py-6">
      {/* Fade edges */}
      <div className="pointer-events-none absolute inset-y-0 left-0 z-10 w-20 sm:w-32 bg-gradient-to-r from-background to-transparent" />
      <div className="pointer-events-none absolute inset-y-0 right-0 z-10 w-20 sm:w-32 bg-gradient-to-l from-background to-transparent" />

      <div className="flex items-center animate-[marquee_26s_linear_infinite] hover:[animation-play-state:paused] w-max">
        {[...TOOLS, ...TOOLS].map((tool, i) => (
          <ToolChip key={`${i}-${tool.name}`} tool={tool} index={i} />
        ))}
      </div>
    </div>
  );
}

/** Backward compat export */
export const TOOL_ICONS: Record<string, () => React.ReactElement> = {};
