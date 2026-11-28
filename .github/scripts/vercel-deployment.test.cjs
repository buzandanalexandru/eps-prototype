// node --test .github/scripts/vercel-deployment.test.cjs
const test = require("node:test");
const assert = require("node:assert");
const wait = require("./vercel-deployment.cjs");

// A fake GitHub: commits newest first, deployments by sha, files changed between commits.
function fake({ commits, deployments, files }) {
  return {
    rest: {
      repos: {
        listCommits: async () => ({ data: commits.map((sha) => ({ sha })) }),
        listDeployments: async ({ sha }) => ({ data: (deployments[sha] || []).map((d, i) => ({ id: `${sha}-${i}`, environment: d.env })) }),
        listDeploymentStatuses: async ({ deployment_id }) => {
          const [sha, i] = deployment_id.split("-");
          const d = deployments[sha][Number(i)];
          return { data: [{ creator: { login: "vercel[bot]" }, state: d.state, environment_url: d.url }] };
        },
        compareCommits: async ({ base, head }) => ({ data: { status: "ahead", files: (files[`${base}..${head}`] || []).map((filename) => ({ filename })) } }),
      },
    },
  };
}
function run(gh, opts) {
  const out = {};
  const core = { info() {}, notice(m) { out.notice = m; }, setOutput(k, v) { out[k] = v; }, setFailed(m) { out.failed = m; } };
  return wait({ github: gh, context: { repo: { owner: "o", repo: "r" } }, core }, { delayMs: 1, attempts: 3, skippedWaits: 2, ...opts }).then(() => out);
}
const ok = (url) => ({ env: "Preview", state: "success", url });

test("services-only commit falls back to the branch's latest ready preview", async () => {
  const gh = fake({ commits: ["c2", "c1"], deployments: { c1: [ok("https://eps-prototype-old.vercel.app")] }, files: { "c1..c2": ["services/x.go", "docs/a.md"] } });
  const out = await run(gh, { sha: "c2", production: false });
  assert.equal(out.url, "https://eps-prototype-old.vercel.app");
  assert.equal(out.commit, "c1");
  assert.match(out.notice, /FALLBACK/);
});

test("a skipped (cancelled) status also falls back", async () => {
  const gh = fake({ commits: ["c2", "c1"], deployments: { c1: [ok("https://eps-prototype-old.vercel.app")], c2: [{ env: "Preview", state: "failure", url: "" }] }, files: { "c1..c2": ["services/x.go"] } });
  const out = await run(gh, { sha: "c2", production: false });
  assert.equal(out.commit, "c1");
  assert.equal(out.failed, undefined);
});

test("a web change waits for this commit's own deployment", async () => {
  const gh = fake({ commits: ["c2", "c1"], deployments: { c1: [ok("https://eps-prototype-old.vercel.app")], c2: [ok("https://eps-prototype-new.vercel.app")] }, files: { "c1..c2": ["apps/web/src/x.ts"] } });
  const out = await run(gh, { sha: "c2", production: false });
  assert.deepEqual([out.url, out.commit], ["https://eps-prototype-new.vercel.app", "c2"]);
});

test("a web change whose deployment failed still fails (no fallback)", async () => {
  const gh = fake({ commits: ["c2", "c1"], deployments: { c1: [ok("https://eps-prototype-old.vercel.app")], c2: [{ env: "Preview", state: "failure", url: "" }] }, files: { "c1..c2": ["api/openapi.yaml"] } });
  const out = await run(gh, { sha: "c2", production: false });
  assert.match(out.failed, /ended "failure"/);
});

test("no earlier ready deployment: strict wait", async () => {
  const gh = fake({ commits: ["c2", "c1"], deployments: {}, files: {} });
  const out = await run(gh, { sha: "c2", production: false });
  assert.match(out.failed, /No successful Vercel preview deployment/);
});

test("production ignores preview deployments and keeps the host check", async () => {
  const gh = fake({ commits: ["m2", "m1"], deployments: { m1: [{ env: "Production", state: "success", url: "https://evil.example.com" }] }, files: { "m1..m2": ["services/x.go"] } });
  const out = await run(gh, { sha: "m2", production: true });
  assert.ok(out.failed, "an ancestor with a foreign URL is never used");
});
