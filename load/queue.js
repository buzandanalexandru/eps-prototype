// k6: the waiting room at opening (step 5, spec "waiting-room" scenario Opening spike): joins and
// status polls against one gateway instance.
//
//   cd services && EPS_ENV=dev DATABASE_URL=... REDIS_URL=... go run ./cmd/loadkit -queue -n 60000 -out /tmp/queue.json
//   k6 run -e KIT=/tmp/queue.json -e GATEWAY=http://localhost:8081 -e JOINS=5000 -e POLLS=10000 load/queue.js
//
// Each client joins once (with its session secret and a precomputed proof of work), then polls its
// status. Thresholds: p95 under 200 ms and errors under 0.1% at the given rates.
//
// A measured target, not a passing claim (step 5 review spec M7): one gateway on the 4-core sandbox
// (shared with k6 and Redis) answered with no errors at every rate, at p95 23 ms (joins) and 13 ms
// (polls) at 1,000 joins and 2,000 polls a second, and saturated near 4,000 requests a second; the
// 5,000 joins and 10,000 polls a second target is measured on the cluster in steps 6 and 12.
import http from "k6/http";
import { check, sleep } from "k6";
import { Rate, Trend } from "k6/metrics";
import exec from "k6/execution";
import { SharedArray } from "k6/data";

// One copy of the kit for every VU (SharedArray), not one per VU.
const kitFile = __ENV.KIT || "/tmp/queue.json";
const clients = new SharedArray("clients", () => JSON.parse(open(kitFile)).clients);
const kit = new SharedArray("meta", () => {
  const k = JSON.parse(open(kitFile));
  delete k.clients;
  return [k];
})[0];
const GW = (__ENV.GATEWAY || "http://localhost:8081").replace(/\/$/, "");
const JOINS = Number(__ENV.JOINS || 5000);
const POLLS = Number(__ENV.POLLS || 10000);
const DURATION = __ENV.DURATION || "10s";

const errors = new Rate("queue_errors");
const joinTime = new Trend("join_ms", true);
const statusTime = new Trend("status_ms", true);

export const options = {
  scenarios: {
    // One scenario: each VU joins until it holds a small pool of its own places, then mostly polls
    // them (VUs share nothing; a poller always has places to poll).
    fans: { executor: "constant-arrival-rate", exec: "fan", rate: JOINS + POLLS, timeUnit: "1s", duration: DURATION, preAllocatedVUs: 400, maxVUs: 3000 },
  },
  thresholds: {
    join_ms: ["p(95)<200"],
    status_ms: ["p(95)<200"],
    queue_errors: ["rate<0.001"],
  },
};

// VIA=proxy sends the calls through the web app's own-origin route (BASE: the web app), with
// k6's per-VU cookie jar holding the guest cookie, as a browser does; else straight to the gateway.
const VIA = __ENV.VIA || "gateway";
const BASE = (__ENV.BASE || "http://localhost:3000").replace(/\/$/, "");
const joinUrl = () => (VIA === "proxy" ? `${BASE}/api/shop/queue/${kit.saleId}/join` : `${GW}/v1/queue/${kit.saleId}/join`);
const statusUrl = () => (VIA === "proxy" ? `${BASE}/api/shop/queue/${kit.saleId}/status` : `${GW}/v1/queue/${kit.saleId}/status`);

// The kit's signing key is new to the gateway: wait until it verifies (it reads the keys the core
// publishes to Redis).
export function setup() {
  const c = clients[0];
  for (let i = 0; i < 40; i++) {
    const r = http.post(joinUrl(), JSON.stringify({ pow: c.pow }), { headers: headers(c) });
    if (r.status === 200) return;
    sleep(0.5);
  }
}

const mine = [];
const share = JOINS / (JOINS + POLLS);

function headers(c, extra) {
  const h = { "X-EPS-Session": c.session, "Content-Type": "application/json", "X-EPS-Tab": "k6" };
  if (VIA !== "proxy") h.Authorization = `Bearer ${c.token}`;
  return Object.assign(h, extra || {});
}

function ok(res) {
  if (VIA !== "proxy") return res.status === 200;
  return res.status === 200 && res.json("status") === 200; // the proxy answers {status, body}
}

function body(res) {
  return VIA === "proxy" ? res.json("body") : res.json();
}

export function fan() {
  if (mine.length < 5 || Math.random() < share) {
    const i = exec.scenario.iterationInTest;
    const c = clients[i % clients.length];
    const res = http.post(joinUrl(), JSON.stringify({ pow: c.pow }), { headers: headers(c, { "X-EPS-Client-UA": "k6 load test (Mozilla/5.0)" }), tags: { name: "join" } });
    joinTime.add(res.timings.duration);
    const good = ok(res);
    errors.add(!good);
    check(res, { joined: () => good });
    if (good && body(res).queueToken) mine.push({ c, qt: body(res).queueToken });
    if (mine.length > 50) mine.shift();
    return;
  }
  const p = mine[Math.floor(Math.random() * mine.length)];
  const res = http.get(statusUrl(), { headers: headers(p.c, { "X-EPS-Queue-Token": p.qt }), tags: { name: "status" } });
  statusTime.add(res.timings.duration);
  const good = ok(res);
  errors.add(!good);
  check(res, { status: () => good });
}
