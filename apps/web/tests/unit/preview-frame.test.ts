// Storefront builder spec "Live preview on desktop and phone": the frame accepts updates only from
// the builder that opened it.
import { describe, expect, it } from "vitest";
import { acceptDraft, acceptFromFrame, selectMessage } from "@/lib/storefront/frame-protocol";
import { doc } from "./storefront-fixtures";

const ORIGIN = "https://eps.example";
const parent = { name: "builder window" };
const msg = (seq: number, extra: Record<string, unknown> = {}) => ({ type: "eps:draft", seq, pageId: doc().pages[0].id, locale: "en", scheme: null, selected: null, document: doc(), ...extra });

describe("preview frame protocol", () => {
  it("accepts a valid draft from its parent on the same origin", () => {
    const d = acceptDraft({ origin: ORIGIN, source: parent, data: msg(1) }, { origin: ORIGIN, parent, lastSeq: 0 });
    expect(d.accept).toBe(true);
  });

  it("ignores a document posted by a page on another origin, or by another window", () => {
    expect(acceptDraft({ origin: "https://evil.example", source: parent, data: msg(1) }, { origin: ORIGIN, parent, lastSeq: 0 })).toEqual({ accept: false, reason: "origin" });
    expect(acceptDraft({ origin: "null", source: parent, data: msg(1) }, { origin: ORIGIN, parent, lastSeq: 0 })).toEqual({ accept: false, reason: "origin" });
    expect(acceptDraft({ origin: ORIGIN, source: { other: true }, data: msg(1) }, { origin: ORIGIN, parent, lastSeq: 0 })).toEqual({ accept: false, reason: "source" });
    expect(acceptDraft({ origin: ORIGIN, source: null, data: msg(1) }, { origin: ORIGIN, parent, lastSeq: 0 })).toEqual({ accept: false, reason: "source" });
  });

  it("ignores stale updates, wrong shapes and invalid documents", () => {
    expect(acceptDraft({ origin: ORIGIN, source: parent, data: msg(3) }, { origin: ORIGIN, parent, lastSeq: 3 })).toEqual({ accept: false, reason: "stale" });
    expect(acceptDraft({ origin: ORIGIN, source: parent, data: { ...msg(4), extra: 1 } }, { origin: ORIGIN, parent, lastSeq: 3 })).toEqual({ accept: false, reason: "shape" });
    expect(acceptDraft({ origin: ORIGIN, source: parent, data: "eps:draft" }, { origin: ORIGIN, parent, lastSeq: 3 })).toEqual({ accept: false, reason: "shape" });
    const bad = doc();
    (bad.pages[0].sections[0].settings as Record<string, unknown>).heading = { ro: "<x>", en: "x", js: "1" };
    expect(acceptDraft({ origin: ORIGIN, source: parent, data: msg(5, { document: bad }) }, { origin: ORIGIN, parent, lastSeq: 3 })).toEqual({ accept: false, reason: "document" });
  });

  it("the builder accepts a select message only from its own frame", () => {
    const frame = { frame: true };
    const data = { type: "eps:select", pageId: "p-1", sectionId: "hero-1" };
    expect(acceptFromFrame({ origin: ORIGIN, source: frame, data }, { origin: ORIGIN, frame }, selectMessage)).toEqual(data);
    expect(acceptFromFrame({ origin: "https://evil.example", source: frame, data }, { origin: ORIGIN, frame }, selectMessage)).toBeNull();
    expect(acceptFromFrame({ origin: ORIGIN, source: {}, data }, { origin: ORIGIN, frame }, selectMessage)).toBeNull();
    expect(acceptFromFrame({ origin: ORIGIN, source: frame, data: { ...data, sectionId: "<script>" } }, { origin: ORIGIN, frame }, selectMessage)).toBeNull();
  });
});
