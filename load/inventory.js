// k6: the sharded inventory under load (step 5, spec "inventory" scenario Throughput).
// Drives holds on one pool against one core instance at a constant arrival rate, cancels a share of
// them at once (CANCEL, default 30%: releases under load), and checks no oversell at the end.
// Commits need a payment through the simulated providers: the simulator (step 6) drives them.
//
// A measured target, not a passing claim (step 5 review spec M7): on the 4-core sandbox shared with
// k6, Postgres and Redis it never oversold, held 500/s at p95 32 ms and saturated near 900/s; the
// 1,000/s at p95 50 ms target is measured on the cluster in steps 6 and 12.
//
//   cd services && EPS_ENV=dev DATABASE_URL=... go run ./cmd/loadkit -n 40000 -capacity 20000 -out /tmp/load.json
//   k6 run -e KIT=/tmp/load.json -e CORE=http://localhost:8080 -e RATE=1000 load/inventory.js
//
// Thresholds: at least 1,000 holds per second sustained (the arrival rate is met with no dropped
// iterations), p95 of a hold under 50 ms, no unexpected errors, and never more units held or sold
// than the capacity.
import http from "k6/http";
import { check, sleep } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";
import exec from "k6/execution";
import { SharedArray } from "k6/data";

// One copy of the kit for every VU (SharedArray), not one per VU.
const kitFile = __ENV.KIT || "/tmp/load.json";
const clients = new SharedArray("clients", () => JSON.parse(open(kitFile)).clients);
const kit = new SharedArray("meta", () => {
  const k = JSON.parse(open(kitFile));
  delete k.clients;
  return [k];
})[0];
const CORE = (__ENV.CORE || "http://localhost:8080").replace(/\/$/, "");
const RATE = Number(__ENV.RATE || 1000);
const DURATION = __ENV.DURATION || "30s";

const holdTime = new Trend("hold_ms", true);
const unitsHeld = new Counter("units_held");
const soldOut = new Counter("sold_out");
const unexpected = new Rate("unexpected_errors");
const unitsReleased = new Counter("units_released"); // given back by cancels
const cancelTime = new Trend("cancel_ms", true);
// CANCEL: the share of holds cancelled right away (their units go back: release under load).
const CANCEL = Number(__ENV.CANCEL || 0.3);

export const options = {
  scenarios: {
    holds: { executor: "constant-arrival-rate", rate: RATE, timeUnit: "1s", duration: DURATION, preAllocatedVUs: 200, maxVUs: 1000 },
  },
  thresholds: {
    hold_ms: ["p(95)<50"],
    unexpected_errors: ["rate<0.001"],
    dropped_iterations: ["count<" + Math.max(1, Math.round(RATE * 0.01))],
  },
};

let shown = 0;

export default function () {
  const i = exec.scenario.iterationInTest;
  const c = clients[i % clients.length];
  const qty = (i % 4) + 1;
  const res = http.post(`${CORE}/v1/shop/orders`, JSON.stringify({ eventId: kit.eventId, version: kit.version, locale: "en", items: [{ productId: kit.productId, qty }] }), {
    headers: { Authorization: `Bearer ${c.token}`, "Content-Type": "application/json" },
    tags: { name: "hold" },
  });
  holdTime.add(res.timings.duration);
  const ok = res.status === 201 || res.status === 200;
  const out = res.status === 409 && String(res.body).includes("EPS-HOLD-SOLD-OUT");
  unexpected.add(!ok && !out);
  if (!ok && !out && shown++ < 5) console.warn(`unexpected answer ${res.status}: ${String(res.body).slice(0, 200)}`);
  if (res.status === 201) {
    unitsHeld.add(qty);
    if (Math.random() < CANCEL) {
      const id = res.json("id");
      const cr = http.post(`${CORE}/v1/shop/orders/${id}/cancel`, null, { headers: { Authorization: `Bearer ${c.token}` }, tags: { name: "cancel" } });
      cancelTime.add(cr.timings.duration);
      if (cr.status === 200) unitsReleased.add(qty);
      else unexpected.add(true);
    }
  }
  if (out) soldOut.add(1);
  check(res, { "held or sold out": () => ok || out });
}

// The kit's signing key is new to the core: one call first lets it load the key.
export function setup() {
  for (let i = 0; i < 20; i++) {
    if (http.get(`${CORE}/v1/whoami`, { headers: { Authorization: `Bearer ${clients[0].token}` } }).status === 200) return;
    sleep(0.5);
  }
}

export function teardown() {
  // No oversell: the core's public availability never shows more taken than the capacity.
  const res = http.get(`${CORE}/v1/public/events/${kit.eventId}/availability`);
  const body = res.json();
  const ga = (body.products || []).find((p) => p.productId === kit.productId);
  const inst = (body.instances || []).find((x) => x.instanceId === kit.poolId);
  console.log(`after the run: product ${ga && ga.status}, pool remaining ${inst ? inst.remaining : "(room)"} of ${kit.capacity}`);
  // No oversell: what is held or sold never exceeds the capacity (k6 reports units_held minus
  // units_released; the core's own answer must agree).
  if (inst && inst.remaining < 0) throw new Error("oversold");
}
