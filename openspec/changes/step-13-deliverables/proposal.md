# Step 13: Deliverables

**Demo act:** all three acts and the architecture close: this step turns the working prototype into the submission (the video of at most 5 minutes and the vision of at most one page).
**Features:** none new. It presents C10 (NIBIRU as organiser #1), the three acts from `feature-scope.md` decision 1, and the numbers from step 12 (I3).
**Decisions:** none new; the vision explains S2, P1, P3, O1–O7, U3, U4, R1–R3 and the load story rule in `architecture.md`.

## Why

The challenge is judged on three things: the prototype, a video of at most 5 minutes and a vision of at most one page. The prototype only counts if the video shows it clearly and the vision answers the four questions asked: what next-gen means and why, what to build in the first 6 months, how the platform survives 100,000 people in 10 minutes with bots and failing payments, and what team is needed. Both must use the same honest numbers as the `/scale` page, and the demo must be repeatable from a clean state.

## What Changes

- `docs/deliverables/video-script.md`: the narrated script in three acts plus the architecture close, with timings (Act 1 about 1:30, Act 2 about 2:00, Act 3 about 1:00, architecture 0:30; hard cap 5:00).
- `docs/deliverables/shot-list.md`: each shot with screen, URL, action, on-screen text and duration.
- `docs/deliverables/recording-plan.md`: tools, screen size, browser profiles, language, takes, backups for live AI and load moments, editing and export.
- `docs/deliverables/demo-reset.md`: how to bring the demo data and the cluster back to the starting state before each take.
- `docs/deliverables/vision.en.md` and `docs/deliverables/vision.ro.md`: the one-page vision in English and Romanian (drafted, then cut to one page).
- `docs/deliverables/submission-checklist.md`: links, accounts for the interviewers, and the final checks.

## Capabilities

### New Capabilities
None: this step changes no system behaviour, so `.openspec.yaml` sets `skip_specs: true`.

### Modified Capabilities
None.

## Impact

- Documents under `docs/deliverables/` only, plus the recorded video file stored outside the repository (linked from the checklist).
- May add a demo reset script under `scripts/` if the reset procedure needs one; it reuses the existing seed and simulator commands.
- No code paths, specs or dependencies change.
