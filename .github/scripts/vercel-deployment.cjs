// Waits for Vercel's deployment of a commit and sets the step output `url` (used by deploy.yml and
// preview.yml through actions/github-script).
//
// Only statuses posted by vercel[bot] count: the jobs that run in the GitHub environment
// "production" create GitHub deployments of their own for the same commit, whose statuses point at
// the job log, and Vercel may post its production status onto one of those (environment
// "production", URL only in target_url, environment_url empty). The newest deployment that carries
// a Vercel status decides: success gives its URL, failure or error fails the job at once, anything
// else waits. The URL must be one of this project's deployments (eps-prototype-*.vercel.app).
//
// Vercel skips building a commit that changes nothing the web app uses (apps/web/vercel.json
// ignoreCommand, deploy/scripts/web-paths.cjs). So first the newest ancestor with a successful
// Vercel deployment in the same environment is found (the branch's latest READY deployment, or the
// current production one on main). If the files changed since that ancestor need no web build,
// this commit's own deployment is used when Vercel made one anyway, else the ancestor's, and the
// fallback is logged. If they do need a build (or there is no such ancestor), the strict wait for
// this commit's deployment applies, exactly as before. Outputs `url` and `commit` (the web commit
// the deployment serves, for EXPECT_WEB_COMMIT).
const { needsWebBuild } = require("../../deploy/scripts/web-paths.cjs");

const HOST = /^eps-prototype(-[a-z0-9-]+)?\.vercel\.app$/;

function deploymentUrl(raw) {
  let u;
  try {
    u = new URL(raw);
  } catch {
    return null;
  }
  const bare =
    !u.username &&
    !u.password &&
    !u.port &&
    !u.search &&
    !u.hash &&
    (u.pathname === "/" || u.pathname === "");
  return u.protocol === "https:" && bare && HOST.test(u.hostname)
    ? u.origin
    : null;
}

// The newest Vercel status of a commit in the environment: { state, url, raw, id } or null.
async function vercelStatus(github, repo, sha, inEnv) {
  const { data: deployments } = await github.rest.repos.listDeployments({ ...repo, sha, per_page: 100 });
  for (const d of deployments.filter((d) => inEnv(d.environment))) {
    const { data: statuses } = await github.rest.repos.listDeploymentStatuses({ ...repo, deployment_id: d.id, per_page: 100 });
    const s = statuses.find((s) => s.creator?.login === "vercel[bot]");
    if (!s) continue;
    const raw = s.environment_url || s.target_url;
    return { state: s.state, raw, url: deploymentUrl(raw), id: d.id };
  }
  return null;
}

// The newest ancestor of sha (first parents, up to 50) with a successful Vercel deployment.
async function readyAncestor(github, repo, sha, inEnv) {
  const { data: commits } = await github.rest.repos.listCommits({ ...repo, sha, per_page: 50 });
  for (const c of commits.slice(1)) {
    const st = await vercelStatus(github, repo, c.sha, inEnv);
    if (st && st.state === "success" && st.url) return { sha: c.sha, url: st.url, id: st.id };
  }
  return null;
}

// Whether the commits between base and head need a web build (unknown: yes).
async function webChanged(github, repo, base, head) {
  try {
    const { data } = await github.rest.repos.compareCommits({ ...repo, base, head, per_page: 300 });
    const files = (data.files || []).map((f) => f.filename);
    if (data.status !== "ahead" || files.length >= 300) return true;
    return needsWebBuild(files);
  } catch {
    return true;
  }
}

module.exports = async (
  { github, context, core },
  { sha, production, attempts = 90, delayMs = 10000, skippedWaits = 9 },
) => {
  const kind = production ? "production" : "preview";
  const inEnv = (e) =>
    production ? /^production$/i.test(e) : !/^production$/i.test(e);
  const repo = { owner: context.repo.owner, repo: context.repo.repo };
  const prev = await readyAncestor(github, repo, sha, inEnv);
  if (prev && !(await webChanged(github, repo, prev.sha, sha))) {
    core.info(`Nothing the web app uses changed since ${prev.sha} (its Vercel ${kind} deployment is ready): Vercel may skip building ${sha}.`);
    for (let i = 0; i < skippedWaits; i++) {
      const st = await vercelStatus(github, repo, sha, inEnv);
      if (st && st.state === "success") {
        if (!st.url)
          return core.setFailed(`Vercel reported ${kind} success for ${sha} with a URL that is not an eps-prototype deployment: ${st.raw}`);
        core.info(`Vercel built ${sha} anyway: ${st.url} (GitHub deployment ${st.id})`);
        core.setOutput("commit", sha);
        return core.setOutput("url", st.url);
      }
      if (st && st.state !== "pending" && st.state !== "in_progress" && st.state !== "queued") break; // skipped or cancelled
      await new Promise((r) => setTimeout(r, delayMs));
    }
    core.notice(`FALLBACK: no Vercel ${kind} deployment of ${sha} (build skipped: no web change). Using the deployment of ${prev.sha}: ${prev.url}`);
    core.setOutput("commit", prev.sha);
    return core.setOutput("url", prev.url);
  }
  core.setOutput("commit", sha);
  for (let i = 0; i < attempts; i++) {
    // Newest first.
    const { data: deployments } = await github.rest.repos.listDeployments({
      ...repo,
      sha,
      per_page: 100,
    });
    for (const d of deployments.filter((d) => inEnv(d.environment))) {
      const { data: statuses } = await github.rest.repos.listDeploymentStatuses(
        { ...repo, deployment_id: d.id, per_page: 100 },
      );
      const s = statuses.find((s) => s.creator?.login === "vercel[bot]");
      if (!s) continue;
      const raw = s.environment_url || s.target_url;
      if (s.state === "success") {
        const url = deploymentUrl(raw);
        if (!url)
          return core.setFailed(
            `Vercel reported ${kind} success for ${sha} with a URL that is not an eps-prototype deployment: ${raw}`,
          );
        core.info(
          `Vercel ${kind} deployment of ${sha}: ${url} (GitHub deployment ${d.id})`,
        );
        return core.setOutput("url", url);
      }
      if (s.state === "failure" || s.state === "error")
        return core.setFailed(
          `Vercel ${kind} deployment of ${sha} ended "${s.state}": ${raw}`,
        );
      break; // the newest Vercel deployment is still building
    }
    await new Promise((r) => setTimeout(r, delayMs));
  }
  core.setFailed(
    `No successful Vercel ${kind} deployment for ${sha} after ${Math.round((attempts * delayMs) / 60000)} minutes. Is the Vercel project linked to the repository?`,
  );
};

module.exports.deploymentUrl = deploymentUrl;
