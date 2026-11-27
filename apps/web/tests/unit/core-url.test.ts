import { describe, expect, it } from "vitest";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";
import { coreBaseUrl, envSlug, gatewayBaseUrl } from "@/lib/core/url";

const root = path.resolve(__dirname, "../../../..");
const vectors: { branch: string; slug: string }[] = JSON.parse(
  readFileSync(path.join(root, "deploy/preview-slug-vectors.json"), "utf8"),
).vectors;

describe("environment slug", () => {
  it("matches the shared vectors", () => {
    for (const v of vectors) expect(envSlug(v.branch), v.branch).toBe(v.slug);
  });

  it("the CI script derives the same slugs", () => {
    for (const v of vectors) {
      const out = execFileSync("bash", [path.join(root, "deploy/scripts/slug.sh"), v.branch]).toString().trim();
      expect(out, v.branch).toBe(v.slug);
    }
  });

  it("symbols and great length give a lowercase DNS label of at most 47 characters", () => {
    const s = envSlug("feature/Squad_Buy-and-a-very-long-description-of-the-change");
    expect(s.length).toBeLessThanOrEqual(47);
    expect(s).toMatch(/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/);
    for (const b of ["x".repeat(300), "-a-", "Ünïcödé/branch", "...", "a".repeat(40) + "/b"]) {
      const slug = envSlug(b);
      expect(slug.length, b).toBeLessThanOrEqual(47);
      expect(slug, b).toMatch(/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/);
    }
  });

  it("distinct long branches sharing a prefix get distinct slugs", () => {
    const p = "feature/a-very-long-shared-prefix-for-two-branches-";
    expect(envSlug(p + "one")).not.toBe(envSlug(p + "two"));
  });
});

describe("core address", () => {
  it("explicit configuration wins and ignores the branch", () => {
    expect(
      coreBaseUrl({ CORE_API_URL: "https://core.prod.1.2.3.4.sslip.io/", VERCEL_GIT_COMMIT_REF: "other", CORE_BASE_DOMAIN: "x" }),
    ).toBe("https://core.prod.1.2.3.4.sslip.io");
  });

  it("two previews each derive their own core and gateway", () => {
    const base = { VERCEL: "1", CORE_BASE_DOMAIN: "1.2.3.4.sslip.io" };
    const a = coreBaseUrl({ ...base, VERCEL_GIT_COMMIT_REF: "step-1-organisers" });
    const b = coreBaseUrl({ ...base, VERCEL_GIT_COMMIT_REF: "step-2-builder" });
    expect(a).toBe("https://core.step-1-organisers.1.2.3.4.sslip.io");
    expect(b).toBe("https://core.step-2-builder.1.2.3.4.sslip.io");
    expect(gatewayBaseUrl({ ...base, VERCEL_GIT_COMMIT_REF: "main" })).toBe("https://gw.prod.1.2.3.4.sslip.io");
  });

  it("on Vercel without configuration there is no core address; locally it is localhost", () => {
    expect(coreBaseUrl({ VERCEL: "1", VERCEL_GIT_COMMIT_REF: "x" })).toBeNull();
    expect(coreBaseUrl({})).toBe("http://localhost:8080");
  });
});
