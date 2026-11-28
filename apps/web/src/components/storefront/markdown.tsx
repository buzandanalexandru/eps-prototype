// Renders organiser text in the formatting subset as React elements: every character is escaped by
// React, so "<script>" or an onerror attribute stays visible text (design D2.2).
import { Fragment } from "react";
import { parseMarkdown, type Inline } from "@/lib/storefront/markdown";
import { shopHref } from "./href";

function inline(xs: Inline[], hrefBase: string, interactive: boolean): React.ReactNode {
  return xs.map((x, i) => {
    if (x.t === "text") return <Fragment key={i}>{x.v}</Fragment>;
    if (x.t === "b") return <strong key={i}>{inline(x.c, hrefBase, interactive)}</strong>;
    if (x.t === "i") return <em key={i}>{inline(x.c, hrefBase, interactive)}</em>;
    const href = shopHref(x.href, hrefBase);
    return (
      <a key={i} href={href} rel={href.startsWith("/") ? undefined : "noopener noreferrer nofollow"} className="underline text-[var(--eps-primary)]" data-eps-link={interactive ? undefined : "inert"}>
        {inline(x.c, hrefBase, interactive)}
      </a>
    );
  });
}

export function Markdown({ source, hrefBase = "", interactive = true, className }: { source: string; hrefBase?: string; interactive?: boolean; className?: string }) {
  return (
    <div className={className}>
      {parseMarkdown(source).map((b, i) =>
        b.t === "p" ? (
          <p key={i} className="whitespace-pre-line [&:not(:first-child)]:mt-3">
            {inline(b.c, hrefBase, interactive)}
          </p>
        ) : (
          <ul key={i} className="ml-5 list-disc space-y-1 [&:not(:first-child)]:mt-3">
            {b.items.map((it, j) => (
              <li key={j}>{inline(it, hrefBase, interactive)}</li>
            ))}
          </ul>
        ),
      )}
    </div>
  );
}
