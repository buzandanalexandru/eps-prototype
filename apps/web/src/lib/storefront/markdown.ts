// The small formatting subset for organiser text (design D2.2): paragraphs, "- " lists, **bold**,
// _italic_ and [links](https://...). It parses to a tree that the renderer turns into React elements,
// so every character is escaped by React; HTML in the source stays visible text. Unsafe links
// render as plain text.
import { isSafeLink } from "./links";

export type Inline = { t: "text"; v: string } | { t: "b"; c: Inline[] } | { t: "i"; c: Inline[] } | { t: "a"; href: string; c: Inline[] };
export type MdBlock = { t: "p"; c: Inline[] } | { t: "ul"; items: Inline[][] };

const TOKEN = /\*\*(.+?)\*\*|_(.+?)_|\[([^\]\n]{1,200})\]\(([^)\s]{1,500})\)/;

export function parseInline(src: string, depth = 0): Inline[] {
  const out: Inline[] = [];
  let rest = src;
  while (rest) {
    const m = depth < 3 ? TOKEN.exec(rest) : null;
    if (!m) {
      out.push({ t: "text", v: rest });
      break;
    }
    if (m.index > 0) out.push({ t: "text", v: rest.slice(0, m.index) });
    if (m[1] !== undefined) out.push({ t: "b", c: parseInline(m[1], depth + 1) });
    else if (m[2] !== undefined) out.push({ t: "i", c: parseInline(m[2], depth + 1) });
    else if (isSafeLink(m[4])) out.push({ t: "a", href: m[4], c: parseInline(m[3], depth + 1) });
    else out.push({ t: "text", v: m[3] });
    rest = rest.slice(m.index + m[0].length);
  }
  return out;
}

export function parseMarkdown(src: string): MdBlock[] {
  const blocks: MdBlock[] = [];
  for (const chunk of src.replace(/\r\n?/g, "\n").split(/\n{2,}/)) {
    const lines = chunk.split("\n").filter((l) => l.trim() !== "");
    if (!lines.length) continue;
    let para: string[] = [];
    let list: string[] = [];
    const flush = () => {
      if (para.length) blocks.push({ t: "p", c: parseInline(para.join("\n")) });
      if (list.length) blocks.push({ t: "ul", items: list.map((l) => parseInline(l)) });
      para = [];
      list = [];
    };
    for (const line of lines) {
      const item = /^\s*[-*]\s+(.*)$/.exec(line);
      if (item) {
        if (para.length) flush();
        list.push(item[1]);
      } else {
        if (list.length) flush();
        para.push(line);
      }
    }
    flush();
  }
  return blocks;
}

/** Plain text of a markdown source (for alt texts, emails' text part, search). */
export function markdownText(src: string): string {
  const walk = (xs: Inline[]): string => xs.map((x) => (x.t === "text" ? x.v : walk(x.c))).join("");
  return parseMarkdown(src)
    .map((b) => (b.t === "p" ? walk(b.c) : b.items.map((i) => `- ${walk(i)}`).join("\n")))
    .join("\n\n");
}
