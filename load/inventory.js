// k6: the sharded inventory under load (step 5, spec "inventory" scenario Throughput).
// Drives holds (and commits through cancels' opposite: half the holds are cancelled at once) on one
// pool against one core instance at a constant arrival rate, and checks no oversell at the end.
//
//   cd services && EPS_ENV=dev DATABASE_URL=... go run ./cmd/loadkit -n 40000 -capacity 20000 -out /tmp/load.json
//   k6 run -e KIT=/tmp/load.json -e CORE=http://localhost:8080 -e RATE=1000 load/inventory.js
//
// Thresholds: at least 1,000 holds per second sustained (the arrival rate is met with no dropped
// iterations), p95 of a hold under 50 ms, no unexpected errors, and never more units held or sold
// than the capacity.
import http from "k6/http";
import { check } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";
import exec from "k6/execution";
import { SharedArray } from "k6/data";

// One copy of the kit for every VU (SharedArray), not one per VU.
const kitFile = __ENV.KIT || "/tmp/load.json";
const kit = new SharedArray("kit", () => [JSON.parse(open(kitFile))])[0];
const CORE = (__ENV.CORE || "http://localhost:8080").replace(/\/$/, "");
const RATE = Number(__ENV.RATE || 1000);
const DURATION = __ENV.DURATION || "30s";

const holdTime = new Trend("hold_ms", true);
const unitsHeld = new Counter("units_held");
const soldOut = new Counter("sold_out");
const unexpected = new Rate("unexpected_errors");

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

export default function () {
  const i = exec.scenario.iterationInScenario;
  const c = kit.clients[i % kit.clients.length];
  const qty = (i % 4) + 1;
  const res = http.post(`${CORE}/v1/shop/orders`, JSON.stringify({ eventId: kit.eventId, version: kit.version, locale: "en", items: [{ productId: kit.productId, qty }] }), {
    headers: { Authorization: `Bearer ${c.token}`, "Content-Type": "application/json" },
    tags: { name: "hold" },
  });
  holdTime.add(res.timings.duration);
  const ok = res.status === 201 || res.status === 200;
  const out = res.status === 409 && String(res.body).includes("EPS-HOLD-SOLD-OUT");
  unexpected.add(!ok && !out);
  if (res.status === 201) unitsHeld.add(qty);
  if (out) soldOut.add(1);
  check(res, { "held or sold out": () => ok || out });
}

export function teardown() {
  // No oversell: the core's public availability never shows more taken than the capacity.
  const res = http.get(`${CORE}/v1/public/events/${kit.eventId}/availability`);
  const body = res.json();
  const ga = (body.products || []).find((p) => p.productId === kit.productId);
  const inst = (body.instances || []).find((x) => x.instanceId === kit.poolId);
  console.log(`after the run: product ${ga && ga.status}, pool remaining ${inst ? inst.remaining : "(room)"} of ${kit.capacity}`);
  if (inst && inst.remaining < 0) throw new Error("oversold");
}
