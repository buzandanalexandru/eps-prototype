// The live-preview protocol between the builder and its frame (design D2.4). The builder posts
// { type: "eps:draft", seq, document, pageId, locale, scheme, selected } to the frame; the frame
// accepts it only from its parent window on the same origin, only with a newer seq and only when the
// shape and the document validate, renders it and answers { type: "eps:rendered", seq, ok }. A click
// on a section in the frame posts { type: "eps:select", pageId, sectionId } back to the builder.
import * as z from "zod";
import { docId, LOCALES, parseDocument, type StorefrontDocument } from "./schema";

export const draftMessage = z.strictObject({
  type: z.literal("eps:draft"),
  seq: z.int().min(0).max(Number.MAX_SAFE_INTEGER),
  pageId: docId,
  locale: z.enum(LOCALES),
  scheme: z.enum(["light", "dark"]).nullable(),
  selected: docId.nullable(),
  document: z.unknown(),
});
export type DraftMessage = Omit<z.infer<typeof draftMessage>, "document"> & { document: StorefrontDocument };

export const selectMessage = z.strictObject({ type: z.literal("eps:select"), pageId: docId, sectionId: docId.nullable() });
export const renderedMessage = z.strictObject({ type: z.literal("eps:rendered"), seq: z.int().min(0), ok: z.boolean() });
export const readyMessage = z.strictObject({ type: z.literal("eps:ready") });

type Incoming = { origin: string; source: unknown; data: unknown };

export type FrameDecision = { accept: true; message: DraftMessage } | { accept: false; reason: "origin" | "source" | "shape" | "stale" | "document" };

/** The frame's check of a message (pure, so it is unit-tested). */
export function acceptDraft(ev: Incoming, expected: { origin: string; parent: unknown; lastSeq: number }): FrameDecision {
  if (ev.origin !== expected.origin) return { accept: false, reason: "origin" };
  if (!ev.source || ev.source !== expected.parent) return { accept: false, reason: "source" };
  const m = draftMessage.safeParse(ev.data);
  if (!m.success) return { accept: false, reason: "shape" };
  if (m.data.seq <= expected.lastSeq) return { accept: false, reason: "stale" };
  const doc = parseDocument(m.data.document);
  if (!doc.ok) return { accept: false, reason: "document" };
  return { accept: true, message: { ...m.data, document: doc.doc } };
}

/** The builder's check of a message from its frame. */
export function acceptFromFrame<T extends z.ZodType>(ev: Incoming, expected: { origin: string; frame: unknown }, schema: T): z.infer<T> | null {
  if (ev.origin !== expected.origin || !ev.source || ev.source !== expected.frame) return null;
  const m = schema.safeParse(ev.data);
  return m.success ? m.data : null;
}
