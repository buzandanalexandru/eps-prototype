// Tickets spec "Private key never reaches the web app": the ticket signing key exists only in the
// core's secret. Nothing in the web app's code, its environment, the scripts that set Vercel's
// environment, or its build output names or carries it.
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";
import { expect, it } from "vitest";

const WEB = path.resolve(__dirname, "../..");
const ROOT = path.resolve(WEB, "../..");

function files(dir: string, out: string[] = []): string[] {
  if (!existsSync(dir)) return out;
  for (const name of readdirSync(dir)) {
    if (name === "node_modules" || name === "cache") continue;
    const p = path.join(dir, name);
    const st = statSync(p);
    if (st.isDirectory()) files(p, out);
    else if (st.size < 5_000_000) out.push(p);
  }
  return out;
}

const SECRET = /TICKET_SIGNING_KEY|TICKET_RETIRED_KEYS/;

it("the web app's code and Vercel environment never name the ticket signing key", () => {
  const bad = [...files(path.join(WEB, "src")), ...files(path.join(WEB, "public")), path.join(ROOT, "deploy/scripts/vercel-env.sh")]
    .filter((f) => existsSync(f) && SECRET.test(readFileSync(f, "utf8")));
  expect(bad).toEqual([]);
  expect(Object.keys(process.env).filter((k) => SECRET.test(k))).toEqual([]);
});

it("only the core's chart values receive it", () => {
  const lines = readFileSync(path.join(ROOT, "deploy/helm/eps/values.yaml"), "utf8").split("\n");
  const at = lines.map((l, i) => (/^\s+ticketKeys:/.test(l) ? i : -1)).filter((i) => i >= 0);
  expect(at).toHaveLength(1);
  const section = lines.slice(0, at[0]).reverse().find((l) => /^[A-Za-z]\w*:/.test(l));
  expect(section).toBe("core:");
  for (const f of readdirSync(path.join(ROOT, "deploy/helm/eps/charts"))) {
    if (f === "common") continue;
    for (const x of files(path.join(ROOT, "deploy/helm/eps/charts", f))) expect(readFileSync(x, "utf8"), x).not.toMatch(/TICKET_SIGNING_KEY/);
  }
});

it("the build output (when built) carries no ticket key", () => {
  const out = path.join(WEB, ".next");
  const bad = files(path.join(out, "server")).concat(files(path.join(out, "static"))).filter((f) => SECRET.test(readFileSync(f, "utf8")));
  expect(bad).toEqual([]);
});
