// node --test deploy/scripts/web-paths.test.cjs
const test = require("node:test");
const assert = require("node:assert");
const { execFileSync } = require("node:child_process");
const path = require("node:path");
const { needsWebBuild } = require("./web-paths.cjs");

test("web app, contract and root package files build", () => {
  for (const p of ["apps/web/src/app/page.tsx", "apps/web/messages/ro.json", "apps/web/vercel.json", "api/openapi.yaml",
    "package.json", "package-lock.json", ".nvmrc"]) {
    assert.equal(needsWebBuild([p]), true, p);
  }
});

test("docs, specs, services, deploy, load and CI alone skip", () => {
  const paths = ["docs/planning/build-plan.md", "openspec/changes/x/tasks.md", "services/internal/live/stream.go",
    "deploy/helm/eps/values.yaml", "deploy/scripts/vercel-ignore.sh", "load/queue.js", ".github/workflows/ci.yml", "README.md", "CLAUDE.md", "scripts/check-contract-drift.sh"];
  assert.equal(needsWebBuild(paths), false);
});

test("one web path among many builds", () => {
  assert.equal(needsWebBuild(["services/cmd/core/main.go", "apps/web/src/lib/core/schema.d.ts"]), true);
});

test("look-alike paths under known directories do not build", () => {
  assert.equal(needsWebBuild(["services/apps/web/x", "docs/api/notes.md", "deploy/package.json"]), false);
});

test("unknown top-level paths build (review D-i)", () => {
  for (const p of ["newpkg/index.ts", "tsconfig.base.json", ".npmrc", "turbo.json"]) assert.equal(needsWebBuild([p]), true, p);
});

test("nothing changed skips", () => assert.equal(needsWebBuild([]), false));

test("the command line prints the verdict", () => {
  const run = (input) => execFileSync("node", [path.join(__dirname, "web-paths.cjs")], { input }).toString().trim();
  assert.equal(run("services/cmd/core/main.go\n"), "skip");
  assert.equal(run("services/cmd/core/main.go\napps/web/package.json\n"), "build");
  assert.equal(run(""), "skip");
});
