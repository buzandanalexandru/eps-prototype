// The waiting room's proof of work (step 5): the page's SHA-256 matches the platform's, and its
// nonce passes the gateway's rule (16 leading zero bits of SHA-256 "<sale>:<sid>:<nonce>").
import { createHash } from "node:crypto";
import { describe, expect, it } from "vitest";
import { sha256Hex, solvePow } from "@/components/storefront/queue-client";

describe("waiting room proof of work", () => {
  it("hashes like node's SHA-256", () => {
    for (const s of ["", "abc", "a".repeat(55), "a".repeat(56), "a".repeat(64), "ș ț ă î â", "x".repeat(1000)]) {
      expect(sha256Hex(s)).toBe(createHash("sha256").update(s, "utf8").digest("hex"));
    }
  });

  it("finds a nonce the gateway accepts", () => {
    const sale = "0d000000-0000-4000-8000-0000000000b1";
    const sid = createHash("sha256").update("session-secret").digest("hex");
    const nonce = solvePow(sale, sid);
    const d = createHash("sha256").update(`${sale}:${sid}:${nonce}`).digest();
    expect(d[0]).toBe(0);
    expect(d[1]).toBe(0);
  });
});
