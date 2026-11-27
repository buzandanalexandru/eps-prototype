// Waits for Vercel's deployment of a commit and sets the step output `url` (used by deploy.yml and
// preview.yml through actions/github-script).
//
// Only statuses posted by vercel[bot] count: the jobs that run in the GitHub environment
// "production" create GitHub deployments of their own for the same commit, whose statuses point at
// the job log, and Vercel may post its production status onto one of those (environment
// "production", URL only in target_url, environment_url empty). The newest deployment that carries
// a Vercel status decides: success gives its URL, failure or error fails the job at once, anything
// else waits. The URL must be one of this project's deployments (eps-prototype-*.vercel.app).

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

module.exports = async (
  { github, context, core },
  { sha, production, attempts = 90, delayMs = 10000 },
) => {
  const kind = production ? "production" : "preview";
  const inEnv = (e) =>
    production ? /^production$/i.test(e) : !/^production$/i.test(e);
  const repo = { owner: context.repo.owner, repo: context.repo.repo };
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
