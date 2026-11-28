// k6: the waiting room at opening (step 5, spec "waiting-room" scenario Opening spike): joins and
// status polls against one gateway instance.
//
//   cd services && EPS_ENV=dev DATABASE_URL=... REDIS_URL=... go run ./cmd/loadkit -queue -n 60000 -out /tmp/queue.json
//   k6 run -e KIT=/tmp/queue.json -e GATEWAY=http://localhost:8081 -e JOINS=5000 -e POLLS=10000 load/queue.js
//
// Each client joins once (with its session secret and a precomputed proof of work), then polls its
// status. Thresholds: p95 under 200 ms and errors under 0.1% at the given rates.
import http from "k6/http";
import { check } from "k6";
import { Rate, Trend } from "k6/metrics";
import exec from "k6/execution";
import { SharedArray } from "k6/data";

// One copy of the kit for every VU (SharedArray), not one per VU.
const kitFile = __ENV.KIT || "/tmp/queue.json";
const kit = new SharedArray("kit", () => [JSON.parse(open(kitFile))])[0];
const GW = (__ENV.GATEWAY || "http://localhost:8081").replace(/\/$/, "");
const JOINS = Number(__ENV.JOINS || 5000);
const POLLS = Number(__ENV.POLLS || 10000);
const DURATION = __ENV.DURATION || "10s";

const errors = new Rate("queue_errors");
const joinTime = new Trend("join_ms", true);
const statusTime = new Trend("status_ms", true);

export const options = {
  scenarios: {
    joins: { executor: "constant-arrival-rate", exec: "join", rate: JOINS, timeUnit: "1s", duration: DURATION, preAllocatedVUs: 300, maxVUs: 2000 },
    polls: { executor: "constant-arrival-rate", exec: "poll", rate: POLLS, timeUnit: "1s", duration: DURATION, startTime: "2s", preAllocatedVUs: 300, maxVUs: 2000 },
  },
  thresholds: {
    join_ms: ["p(95)<200"],
    status_ms: ["p(95)<200"],
    queue_errors: ["rate<0.001"],
  },
};

// Queue tokens of the clients this VU joined (k6 VUs share nothing; each polls its own joins).
const mine = [];

function headers(c, extra) {
  return Object.assign({ Authorization: `Bearer ${c.token}`, "X-EPS-Session": c.session, "Content-Type": "application/json" }, extra || {});
}

export function join() {
  const i = exec.scenario.iterationInScenario;
  const c = kit.clients[i % kit.clients.length];
  const res = http.post(`${GW}/v1/queue/${kit.saleId}/join`, JSON.stringify({ pow: c.pow }), { headers: headers(c, { "X-EPS-Client-UA": "k6 load test (Mozilla/5.0)" }), tags: { name: "join" } });
  joinTime.add(res.timings.duration);
  const ok = res.status === 200;
  errors.add(!ok);
  check(res, { "joined": () => ok });
  if (ok) {
    const qt = res.json("queueToken");
    if (qt) mine.push({ c, qt });
  }
}

export function poll() {
  if (mine.length === 0) {
    join();
    return;
  }
  const p = mine[exec.scenario.iterationInScenario % mine.length];
  const res = http.get(`${GW}/v1/queue/${kit.saleId}/status`, { headers: headers(p.c, { "X-EPS-Queue-Token": p.qt }), tags: { name: "status" } });
  statusTime.add(res.timings.duration);
  const ok = res.status === 200;
  errors.add(!ok);
  check(res, { "status": () => ok });
}
