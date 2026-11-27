# Tasks

## 1. Demo reset

- [ ] 1.1 Write `docs/deliverables/demo-reset.md` (Neon demo branch reset, Redis keys of the demo organiser, provider knobs, simulator Jobs, opening time, browser profiles, start URLs, 2-minute check list), adding a reset script under `scripts/` only if needed; verify by running the reset twice in a row and passing the check list both times

## 2. Video

- [ ] 2.1 Draft `docs/deliverables/video-script.md` with narration and timings for the cold open, three acts and the architecture close, using the `/scale` labels; verify the timing column sums to 4:50 or less
- [ ] 2.2 Write `docs/deliverables/shot-list.md` (screen, URL, action, on-screen text, duration per shot) and `docs/deliverables/recording-plan.md` (settings, profiles, per-act takes, labelled backups, export); verify every shot maps to a working URL on production
- [ ] 2.3 Rehearse the full run once on production after a reset with a stopwatch; verify under 5:00 and note cuts in the script if not
- [ ] 2.4 Record the backups (copilot run, simulator run) and then the four takes; verify each take shows the behaviour it claims with no console errors on screen
- [ ] 2.5 Edit, add captions, export MP4 and upload unlisted; verify the final file is 5:00 or less and plays from the link in a private window

## 3. Vision

- [ ] 3.1 Draft `docs/deliverables/vision.en.md` answering the four questions (next-gen and why, first 6 months, 100k in 10 minutes with bots and failing payments, team), with numbers taken from `/scale`; verify every number matches the stored results
- [ ] 3.2 Cut the English vision to one A4 page; verify by exporting to PDF and checking the page count is 1
- [ ] 3.3 Translate to `docs/deliverables/vision.ro.md` with correct diacritics (ă, â, î, ș, ț with comma below); verify one A4 page in PDF and a diacritics check finds no cedilla forms (ş, ţ)

## 4. Submission

- [ ] 4.1 Write `docs/deliverables/submission-checklist.md` (production URL, video link, vision PDFs, repo link, demo organiser sign-in for interviewers, what is simulated); verify every link opens in a private window
- [ ] 4.2 Final consistency check: script, vision and `/scale` use the same numbers and labels, sample data is labelled in every shot, no secret or personal data appears in the video or documents; verify with a recorded checklist in the PR
- [ ] 4.3 Open the PR `step-13-deliverables` with the documents, have one adversarial reviewer check every claim against the prototype and `/scale`, fix findings, and squash-merge; verify CI green and the merged documents render on GitHub

## Workflow follow-up

- Archive with `/opsx:archive step-13-deliverables` (no specs to merge), add `## Status` to the proposal, tick step 13 in `docs/planning/build-plan.md`.
