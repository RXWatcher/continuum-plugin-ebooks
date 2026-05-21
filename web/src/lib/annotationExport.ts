import type { Annotation, EbookDetail } from "./api";

// Markdown export for ebook annotations. Output structure:
//
//   # <Title> — <Authors>
//
//   *Exported YYYY-MM-DD*
//
//   ## Highlight (HH:MM)
//   > <selected_text>
//
//   <note_text>
//
//   ---
//
// Bookmark-only entries (no selected_text) become an italic anchor.
// Style + colour metadata renders as inline tags before the quote so
// the export is grep-able by colour ("blue highlights only").

const STYLE_PRETTY: Record<string, string> = {
  highlight: "Highlight",
  underline: "Underline",
  squiggly: "Squiggly",
};

export function annotationsToMarkdown(
  book: EbookDetail,
  annotations: Annotation[],
): string {
  const lines: string[] = [];
  const title = book.title || "Untitled";
  const authors = book.authors?.join(", ") || "";
  lines.push(`# ${title}${authors ? ` — ${authors}` : ""}`);
  lines.push("");
  lines.push(`*Exported ${new Date().toISOString().slice(0, 10)}*`);
  lines.push("");

  // Stable sort: by CFI range when present (CFI sorts approximately
  // by reading order), then by created_at for ties. Annotations
  // without CFI sink to the bottom — they're typically legacy
  // imports without precise positions.
  const sorted = [...annotations].sort((a, b) => {
    const ac = a.cfi_range || "~";
    const bc = b.cfi_range || "~";
    if (ac !== bc) return ac < bc ? -1 : 1;
    return (a.created_at || "") < (b.created_at || "") ? -1 : 1;
  });

  for (const a of sorted) {
    if (a.deleted_at) continue;
    const tags = formatTags(a);
    const heading = STYLE_PRETTY[a.style || ""] || (a.selected_text ? "Highlight" : "Bookmark");
    lines.push(`## ${heading}${tags ? ` — ${tags}` : ""}`);
    lines.push("");
    if (a.selected_text) {
      // Convert each line of the excerpt into a blockquote line so
      // multi-line highlights render correctly in Markdown.
      for (const line of a.selected_text.split(/\r?\n/)) {
        lines.push(`> ${line}`);
      }
      lines.push("");
    }
    if (a.note_text) {
      lines.push(a.note_text);
      lines.push("");
    }
    if (a.page) {
      lines.push(`*Page ${a.page}*`);
      lines.push("");
    }
    lines.push("---");
    lines.push("");
  }

  return lines.join("\n").trim() + "\n";
}

// formatTags collects colour + readest_type into a comma-separated
// inline tag string. Empty when the annotation has no metadata of
// interest; the calling line then omits the " — " separator.
function formatTags(a: Annotation): string {
  const tags: string[] = [];
  if (a.color) tags.push(a.color);
  if (a.readest_type && a.readest_type !== "annotation") tags.push(a.readest_type);
  return tags.join(", ");
}

// downloadMarkdown writes the Markdown to a Blob and triggers a
// browser download. Filename is the book title sanitised for
// filesystems plus .md.
export function downloadMarkdown(book: EbookDetail, annotations: Annotation[]) {
  const md = annotationsToMarkdown(book, annotations);
  const filename = (book.title || "annotations")
    .replace(/[\\/:*?"<>|]+/g, "_")
    .slice(0, 80) + ".md";
  const blob = new Blob([md], { type: "text/markdown;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
