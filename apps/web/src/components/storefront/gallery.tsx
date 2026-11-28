"use client";
// Gallery island: a grid or a scrolling strip; a click opens a larger view with previous and next.
import { useState } from "react";

type Img = { id: string; src: string; alt: string; sample: boolean };
type Labels = { close: string; previous: string; next: string; sample: string };

export function Gallery({ images, layout, labels }: { images: Img[]; layout: "grid" | "carousel"; labels: Labels }) {
  const [open, setOpen] = useState<number | null>(null);
  if (!images.length) return null;
  const cur = open === null ? null : images[open];
  return (
    <>
      <ul className={layout === "grid" ? "grid grid-cols-2 gap-3 sm:grid-cols-3" : "flex snap-x gap-3 overflow-x-auto pb-2"}>
        {images.map((im, i) => (
          <li key={im.id} data-eps-block={im.id} className={layout === "grid" ? "min-w-0" : "w-64 shrink-0 snap-start sm:w-80"}>
            <button type="button" className="relative block w-full overflow-hidden rounded-[var(--eps-radius)]" onClick={() => setOpen(i)} aria-label={im.alt || labels.sample}>
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src={im.src} alt={im.alt} className="aspect-[4/3] w-full object-cover" loading="lazy" />
              {im.sample && <span className="absolute bottom-1 left-1 rounded bg-[var(--eps-surface)] px-1.5 text-[10px] text-[var(--eps-on-surface)]">{labels.sample}</span>}
            </button>
          </li>
        ))}
      </ul>
      {cur && (
        <div
          role="dialog"
          aria-modal="true"
          aria-label={cur.alt || labels.sample}
          className="fixed inset-0 z-50 flex flex-col items-center justify-center gap-3 bg-[color-mix(in_srgb,var(--eps-surface)_92%,transparent)] p-4"
          onKeyDown={(e) => {
            if (e.key === "Escape") setOpen(null);
            if (e.key === "ArrowRight") setOpen((open! + 1) % images.length);
            if (e.key === "ArrowLeft") setOpen((open! - 1 + images.length) % images.length);
          }}
        >
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src={cur.src} alt={cur.alt} className="max-h-[75vh] max-w-full rounded-[var(--eps-radius)] object-contain" />
          <div className="flex gap-2">
            <button type="button" className="rounded-[var(--eps-radius)] border border-[var(--eps-on-surface)] px-3 py-2 text-sm" onClick={() => setOpen((open! - 1 + images.length) % images.length)}>
              {labels.previous}
            </button>
            <button type="button" autoFocus className="rounded-[var(--eps-radius)] bg-[var(--eps-primary)] px-3 py-2 text-sm text-[var(--eps-on-primary)]" onClick={() => setOpen(null)}>
              {labels.close}
            </button>
            <button type="button" className="rounded-[var(--eps-radius)] border border-[var(--eps-on-surface)] px-3 py-2 text-sm" onClick={() => setOpen((open! + 1) % images.length)}>
              {labels.next}
            </button>
          </div>
        </div>
      )}
    </>
  );
}
