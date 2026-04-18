import type { ReactNode } from "react";

/**
 * Renders message content with basic markdown-like formatting:
 * - URLs become clickable links (images rendered inline)
 * - **bold** text
 * - `code` backticks
 * - #channel references link to /notifications/<name>
 * - @mentions link to agent detail page
 * - [file:ID] attachment references rendered as inline images or download links
 */
export function MessageContent({
  content,
  agentNames,
}: {
  content: string;
  agentNames?: Set<string>;
}) {
  return <>{parseContent(content, agentNames)}</>;
}

const IMAGE_EXT = /\.(png|jpg|jpeg|gif|webp|svg)(\?|$)/i;

/** Tokenize and render inline formatting. */
function parseContent(text: string, agentNames?: Set<string>): ReactNode[] {
  // Split on patterns we want to handle, preserving delimiters
  // Order matters: file refs, URLs first (greedy), then bold, then code, then #notification source, then @mention
  const pattern =
    /(\[file:[a-zA-Z0-9_-]+\])|(https?:\/\/[^\s<>)"']+)|(\*\*(?:[^*]|\*(?!\*))+\*\*)|(`[^`]+`)|(\B#(?=[a-zA-Z0-9_-]*[a-zA-Z])[a-zA-Z0-9_-]+\b)|(@[a-zA-Z0-9_-]+)/g;

  const nodes: ReactNode[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = pattern.exec(text)) !== null) {
    // Push preceding plain text
    if (match.index > lastIndex) {
      nodes.push(text.slice(lastIndex, match.index));
    }

    const [full] = match;
    const key = `${match.index}`;

    if (match[1]) {
      // [file:ID] attachment reference
      const fileId = full.slice(6, -1); // strip [file: and ]
      const fileUrl = `/api/files/${encodeURIComponent(fileId)}`;
      nodes.push(
        <a
          key={key}
          href={fileUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-block my-1"
        >
          <img
            src={fileUrl}
            alt="attachment"
            className="max-w-sm max-h-64 rounded border border-bc-border"
            loading="lazy"
            onError={(e) => {
              // If not an image, show as download link
              const el = e.currentTarget;
              const parent = el.parentElement;
              if (parent) {
                parent.className = "text-bc-accent underline-offset-2 hover:underline text-xs";
                parent.textContent = `📎 ${fileId}`;
              }
            }}
          />
        </a>,
      );
    } else if (match[2]) {
      // URL — render images inline
      const url = full;
      if (IMAGE_EXT.test(url)) {
        nodes.push(
          <a key={key} href={url} target="_blank" rel="noopener noreferrer" className="inline-block my-1">
            <img src={url} alt="" className="max-w-sm max-h-64 rounded border border-bc-border" loading="lazy" />
          </a>,
        );
      } else {
        nodes.push(
          <a
            key={key}
            href={url}
            target="_blank"
            rel="noopener noreferrer"
            className="text-bc-accent underline-offset-2 hover:underline"
          >
            {url}
          </a>,
        );
      }
    } else if (match[3]) {
      // Bold **text**
      const inner = full.slice(2, -2);
      nodes.push(<strong key={key}>{inner}</strong>);
    } else if (match[4]) {
      // Inline code `text`
      const inner = full.slice(1, -1);
      nodes.push(
        <code
          key={key}
          className="rounded bg-bc-surface px-1 py-0.5 font-mono text-[0.85em]"
        >
          {inner}
        </code>,
      );
    } else if (match[5]) {
      // #channel reference → link to /notifications/<name>
      const sourceName = full.slice(1);
      nodes.push(
        <a
          key={key}
          href={`/notifications/${sourceName}`}
          className="text-bc-accent font-medium hover:underline"
        >
          {full}
        </a>,
      );
    } else if (match[6]) {
      // @mention → link to agent detail page, highlight known agents
      const name = full.slice(1);
      const isKnown = agentNames ? agentNames.has(name) : true;
      nodes.push(
        <a
          key={key}
          href={`/agents/${name}`}
          className={
            isKnown
              ? "text-bc-accent font-medium hover:underline bg-bc-accent/10 rounded px-0.5"
              : "text-bc-muted/60 font-medium hover:underline"
          }
        >
          {full}
        </a>,
      );
    }

    lastIndex = match.index + full.length;
  }

  // Push trailing plain text
  if (lastIndex < text.length) {
    nodes.push(text.slice(lastIndex));
  }

  return nodes;
}
