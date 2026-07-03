import fs from "fs";
import path from "path";

export interface DocArticle {
  slug: string;
  title: string;
  description: string;
  content: string;
}

export interface DocsSection {
  id: string;
  label: string;
  description: string;
  articles: DocArticle[];
}

/**
 * Extract the title from the first `# heading` in a markdown file.
 * Falls back to the filename if no heading is found.
 */
function extractTitle(content: string, filename: string): string {
  const match = content.match(/^#\s+(.+)$/m);
  return match
    ? match[1].trim()
    : filename
        .replace(/\.md$/, "")
        .replace(/[-_]/g, " ")
        .replace(/\b\w/g, (c) => c.toUpperCase());
}

/**
 * Extract the first paragraph after the title as a description.
 */
function extractDescription(content: string): string {
  const lines = content.split("\n");
  const titleIdx = lines.findIndex((l) => l.startsWith("# "));
  if (titleIdx === -1) return "";
  for (let i = titleIdx + 1; i < lines.length; i++) {
    const line = lines[i].trim();
    if (!line) continue;
    if (line.startsWith("#") || line.startsWith("```") || line.startsWith(">"))
      break;
    return line;
  }
  return "";
}

/**
 * Read all markdown files from a directory, skipping index.md.
 */
function readDocsDir(dirPath: string, exclude: string[] = []): DocArticle[] {
  if (!fs.existsSync(dirPath)) return [];

  const files = fs
    .readdirSync(dirPath)
    .filter(
      (f) => f.endsWith(".md") && f !== "index.md" && !exclude.includes(f),
    )
    .sort();

  return files.map((file) => {
    const content = fs.readFileSync(path.join(dirPath, file), "utf-8");
    return {
      slug: file.replace(/\.md$/, ""),
      title: extractTitle(content, file),
      description: extractDescription(content),
      content,
    };
  });
}

/**
 * Section metadata with display order.
 */
const SECTIONS: {
  id: string;
  label: string;
  description: string;
  dir: string;
  exclude?: string[];
}[] = [
  {
    id: "tutorials",
    label: "Tutorials",
    description:
      "Step-by-step guides to get you up and running with mycel from scratch.",
    dir: "tutorials",
  },
  {
    id: "how-to",
    label: "How-To Guides",
    description:
      "Practical recipes for common tasks like configuration, channels, and troubleshooting.",
    dir: "how-to",
  },
  {
    id: "reference",
    label: "Reference",
    description:
      "Complete API and CLI reference documentation for every command and endpoint.",
    dir: "reference",
  },
  {
    id: "explanation",
    label: "Explanation",
    description:
      "In-depth technical explanations of architecture, design decisions, and internals.",
    dir: "explanation",
    // Internal engineering docs — published on GitHub, not the marketing site.
    exclude: ["design-system.md", "agent-lifecycle-redesign.md", "ci-cd.md"],
  },
];

/**
 * Load all documentation sections from the docs/ directory.
 * Reads files at build time for static export.
 */
export function loadAllDocs(): DocsSection[] {
  const docsRoot = path.join(process.cwd(), "..", "docs");

  return SECTIONS.map((section) => ({
    id: section.id,
    label: section.label,
    description: section.description,
    articles: readDocsDir(path.join(docsRoot, section.dir), section.exclude),
  }));
}
