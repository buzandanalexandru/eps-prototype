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

  it("no branch other than main maps to prod or to a reserved name", () => {
    for (const b of ["prod", "Prod", "PROD/", "-prod-", "prod/", "staging", "eps", "kube-system", "main/x", "Main"]) {
      const s = envSlug(b);
      expect(s, b).not.toBe("prod");
      expect(s, b).toMatch(/-[0-9a-f]{6}$/);
      expect(["prod", "main", "staging", "eps"]).not.toContain(s.replace(/-[0-9a-f]{6}$/, ""));
      expect(s.startsWith("kube-"), b).toBe(false);
    }
  });

  it("branches that clean to the same text still get different slugs", () => {
    const slugs = ["feature/x", "feature-x", "Feature_X", "feature.x"].map(envSlug);
    expect(new Set(slugs).size).toBe(slugs.length);
  });

  it("non-ASCII letters never fold into ASCII ones", () => {
    // "İ".toLowerCase() is "i̇" in JavaScript; stripping first keeps TS and bash identical.
    expect(envSlug("fix/İnfo")).toMatch(/^fix-nfo-[0-9a-f]{6}$/);
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
    expect(a).toBe(`https://core.${envSlug("step-1-organisers")}.1.2.3.4.sslip.io`);
    expect(b).toBe(`https://core.${envSlug("step-2-builder")}.1.2.3.4.sslip.io`);
    expect(a).not.toBe(b);
    expect(gatewayBaseUrl({ ...base, VERCEL_GIT_COMMIT_REF: "main" })).toBe("https://gw.prod.1.2.3.4.sslip.io");
  });

  it("two pull requests at once: separate core, namespace, database branch and Vercel variable scope", () => {
    const names = (branch: string) =>
      Object.fromEntries(
        execFileSync("bash", [path.join(root, "deploy/scripts/preview-names.sh"), branch])
          .toString()
          .trim()
          .split("\n")
          .map((l) => [l.slice(0, l.indexOf("=")), l.slice(l.indexOf("=") + 1)]),
      ) as Record<string, string>;
    const a = names("step-1-organisers");
    const b = names("step-2-builder");
    for (const key of ["slug", "namespace", "release", "neon_branch", "vercel_git_branch"]) {
      expect(a[key], key).not.toBe(b[key]);
    }
    // The pipeline and the web app agree on each preview's slug, so each web preview talks only to
    // its own core, and the core only to its own CI database branch.
    expect(a.slug).toBe(envSlug("step-1-organisers"));
    expect(a.namespace).toBe(`eps-${a.slug}`);
    // CI's branch is a child of the empty seed, never the integration's copy of production.
    expect(a.neon_branch).toBe(`eps-preview/${a.slug}`);
    expect(a.integration_branch).toBe("preview/step-1-organisers");
    // Even two branches that clean to the same text stay apart.
    expect(names("feature/x").neon_branch).not.toBe(names("feature-x").neon_branch);
  });

  it("the preview pipeline refuses to name anything after production", () => {
    expect(() => execFileSync("bash", [path.join(root, "deploy/scripts/preview-names.sh"), "main"], { stdio: "pipe" })).toThrow();
    for (const b of ["prod", "PROD", "eps"]) {
      const out = execFileSync("bash", [path.join(root, "deploy/scripts/preview-names.sh"), b]).toString();
      expect(out).not.toMatch(/^namespace=eps-prod$/m);
      expect(out).not.toMatch(/^namespace=eps$/m);
    }
  });

  it("on Vercel without configuration there is no core address; locally it is localhost", () => {
    expect(coreBaseUrl({ VERCEL: "1", VERCEL_GIT_COMMIT_REF: "x" })).toBeNull();
    expect(coreBaseUrl({})).toBe("http://localhost:8080");
  });
});
