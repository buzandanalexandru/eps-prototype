import { describe, expect, it } from "vitest";
import { classifyHost } from "@/lib/host";

describe("host classification", () => {
  it("platform hosts route by path", () => {
    for (const h of ["localhost:3000", "eps-prototype.vercel.app", "eps-prototype-git-x-devblocks.vercel.app", "127.0.0.1"])
      expect(classifyHost(h).kind, h).toBe("platform");
    expect(classifyHost("eps.example.com", "eps.example.com").kind).toBe("platform");
  });

  it("any other host is a candidate brand domain", () => {
    expect(classifyHost("tickets.nibiru.ro:443")).toEqual({ kind: "brand", host: "tickets.nibiru.ro" });
    expect(classifyHost("vercel.app.evil.example").kind).toBe("brand");
  });
});
