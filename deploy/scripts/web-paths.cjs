// Which changed paths need a new build of the web app (Vercel's ignored build step and the CI jobs
// that wait for Vercel use the same rule). A build is needed when a commit touches the web app,
// the API contract it generates its types from, or the root package files; docs, specs, the Go
// services, the cluster, load scripts and CI alone do not change what Vercel would build.
//
//   git diff --name-only A B | node deploy/scripts/web-paths.cjs   -> prints "build" or "skip"

const BUILD = [/^apps\/web\//, /^api\//, /^package\.json$/, /^package-lock\.json$/, /^\.nvmrc$/];

/** True when any of the paths needs a web build. */
function needsWebBuild(paths) {
  return paths.some((p) => BUILD.some((re) => re.test(p.trim().replace(/^\.\//, ""))));
}

module.exports = { needsWebBuild };

if (require.main === module) {
  let input = "";
  process.stdin.setEncoding("utf8");
  process.stdin.on("data", (c) => (input += c));
  process.stdin.on("end", () => {
    const paths = input.split("\n").filter((l) => l.trim() !== "");
    process.stdout.write(needsWebBuild(paths) ? "build\n" : "skip\n");
  });
}
