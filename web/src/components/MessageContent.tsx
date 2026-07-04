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
  return <>{parseContent(normalizePlatformMrkdwn(content), agentNames)}</>;
}

/**
 * Rewrites Slack (and Slack-lookalike) mrkdwn angle-bracket syntax into a
 * form the existing tokenizer can render:
 *
 *   <@U0AP1U92T3K>          → @user
 *   <@U0AP1U92T3K|name>     → @name
 *   <#C0BAJV8UXLL|general>  → #general
 *   <#C0BAJV8UXLL>          → #channel
 *   <https://foo|label>     → label (https://foo)  — kept as raw text so
 *                             the URL still gets picked up by the URL
 *                             matcher below without a proper link parser
 *   <https://foo>           → https://foo
 *
 * Slack sends these tokens verbatim in the message payload, which looks
 * broken in the UI (raw user IDs, unclickable channel refs). Fully
 * resolving user IDs to display names needs a Slack users.info round
 * trip and belongs on the server; this client-side pass at least
 * strips the noise.
 */
function normalizePlatformMrkdwn(text: string): string {
  return text
    // <@USERID|label> → @label; <@USERID> → @user
    .replace(/<@[A-Z0-9]+\|([^>]+)>/g, "@$1")
    .replace(/<@[A-Z0-9]+>/g, "@user")
    // <#CHANNELID|name> → #name; <#CHANNELID> → #channel
    .replace(/<#[A-Z0-9]+\|([^>]+)>/g, "#$1")
    .replace(/<#[A-Z0-9]+>/g, "#channel")
    // <https://url|label> → label (https://url); <https://url> → https://url
    .replace(/<(https?:\/\/[^|>]+)\|([^>]+)>/g, "$2 ($1)")
    .replace(/<(https?:\/\/[^>]+)>/g, "$1");
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
            className="max-w-sm max-h-64 rounded border border-mycel-border"
            loading="lazy"
            onError={(e) => {
              // If not an image, show as download link
              const el = e.currentTarget;
              const parent = el.parentElement;
              if (parent) {
                parent.className = "text-mycel-accent underline-offset-2 hover:underline text-xs";
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
            <img src={url} alt="" className="max-w-sm max-h-64 rounded border border-mycel-border" loading="lazy" />
          </a>,
        );
      } else {
        nodes.push(
          <a
            key={key}
            href={url}
            target="_blank"
            rel="noopener noreferrer"
            className="text-mycel-accent underline-offset-2 hover:underline"
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
          className="rounded bg-mycel-surface px-1 py-0.5 font-mono text-[0.85em]"
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
          className="text-mycel-accent font-medium hover:underline"
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
              ? "text-mycel-accent font-medium hover:underline bg-mycel-accent-subtle rounded px-0.5"
              : "text-mycel-muted font-medium hover:underline"
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
