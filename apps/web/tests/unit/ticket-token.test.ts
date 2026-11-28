// Ticket token spec "Same test vectors in Go and the browser": the web verifier accepts the Go
// signer's valid vectors and rejects the altered ones exactly as Go does, with WebCrypto Ed25519 and
// with the library fallback.
import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { decodeCode, fromBase64Url, verifyCode } from "@/lib/tickets/token";

type Vec = { name: string; code: string; eventId?: string; ticketId?: string; generation?: number; window?: number; error?: string };
const vectors: { publicKey: string; keyId: number; valid: Vec[]; invalid: Vec[] } = JSON.parse(
  readFileSync(path.resolve(__dirname, "../../../../services/internal/tickets/testdata/vectors.json"), "utf8"),
);
const keys = new Map([[vectors.keyId, fromBase64Url(vectors.publicKey)!]]);

describe.each([false, true])("ticket codes (fallback only: %s)", (fallbackOnly) => {
  it("accepts every valid Go vector and decodes its fields", async () => {
    for (const v of vectors.valid) {
      const r = await verifyCode(v.code, keys, { fallbackOnly });
      expect(r.ok, v.name).toBe(true);
      if (r.ok) expect(r.token).toEqual({ keyId: 1, eventId: v.eventId, ticketId: v.ticketId, generation: v.generation, window: v.window });
    }
  });
  it("rejects every altered vector for the same reason as Go", async () => {
    const reason: Record<string, string> = { signature: "signature", unknown_key: "unknown_key", malformed: "malformed" };
    for (const v of vectors.invalid) {
      const r = await verifyCode(v.code, keys, { fallbackOnly });
      expect(r.ok, v.name).toBe(false);
      if (!r.ok) expect(r.reason, v.name).toBe(reason[v.error!]);
    }
  });
});

it("a code is about 143 characters and holds no personal data", () => {
  const v = vectors.valid[0];
  expect(v.code.length).toBeLessThanOrEqual(145);
  expect(Object.keys(decodeCode(v.code)!.token).sort()).toEqual(["eventId", "generation", "keyId", "ticketId", "window"]);
});
