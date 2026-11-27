# Step 12: Scale proof

**Demo act:** Act 2 (the k6 numbers and autoscaling shown next to the live simulator) and the architecture close of the video; it carries the answer to "how does the platform survive 100,000 people in 10 minutes, with bots and failing payments". This step is protected in the build plan.
**Features:** I2 (Thin in depth, but it must exist and deploy), I3 (Real), R4 roll-up, G6 (Vision, roadmap only).
**Decisions:** R1 (Helm, autoscaling, CI), R2 (metrics), R3 (simulator), R4 (tests), O1, O2, O3 (the on-sale mechanisms being measured), and the "load story" rule in `architecture.md`.

## Why

The challenge asks how EPS survives a 100k-in-10-minutes on-sale. A live simulator (step 6) shows the mechanisms working, but an interviewer will ask "how many of those can one instance take, and how many do you need?". The answer must be measured, written down with its method and turned into simple maths, never a claim of a real 100k-client test (`competitors/festival-tech.md` §10: size the problem, rehearse, observe; load tests lie unless the method is honest). The roadmap page closes the scope story: what is next and what was cut on purpose.

## What Changes

- **Helm and autoscaling (I2, R1):** the umbrella chart finished for every Go service with resource requests and limits, probes, disruption budgets, and horizontal autoscaling on CPU and requests per second for the gateway and core API; deployed by CI to DOKS fra1.
- **Measured load tests (I3):** k6 scenarios for queue arrival, admitted checkout, payment failover and autoscaling, run inside the cluster against one instance to get per-instance throughput, with results and their method committed and shown unchanged.
- **Scaling maths and the scale page:** a public page that shows the measured numbers, the in-system simulator, and the maths from per-instance throughput to 100k fans in 10 minutes, each labelled measured, simulated or calculated.
- **Roadmap page:** a public page listing the Vision items (G6 refunds and cancellation) and, as context, the cut and out-of-scope items, with no links to screens that do not exist.
- **Test roll-up (R4):** a check that every scenario in every spec names a test that exists, and a roll-up of test counts and results shown on the scale page.

## Capabilities

### New Capabilities
- `scale-proof`: the deployable chart and autoscaling, the measured load results and their method, the scaling maths and scale page, the roadmap page and the test roll-up.

### Modified Capabilities
None.

## Impact

- **deploy/helm/eps:** sub-chart values, HPA, PDB, probes, resources, Prometheus Adapter values for the requests-per-second metric.
- **load/:** k6 scripts, an in-cluster k6 Job manifest, committed results in `load/results/`.
- **apps/web:** `/scale` and `/roadmap` pages (RO and EN) reading a generated results file.
- **CI:** a load-test workflow run by hand (never on every PR), the spec coverage check on every PR.
- **Dependencies:** Prometheus Adapter (Helm chart) for the custom HPA metric, added to the cluster's charts; k6 is already in the stack.
