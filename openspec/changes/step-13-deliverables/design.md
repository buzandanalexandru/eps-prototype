# Design: step 13, deliverables

## Context

See `proposal.md` for why. By now the deployed prototype has the three acts (steps 1–8), the extras (steps 9–11) and the `/scale` and `/roadmap` pages with measured numbers (step 12). The interviewers see the video first, then may click the public URL.

## Goals / Non-Goals

**Goals:**
- A video that shows real behaviour, not slides, in under 5 minutes, and can be re-recorded in under an hour from a clean state.
- A vision that fits one A4 page in each language and makes no claim the prototype or the measurements do not back.

**Non-Goals:**
- Marketing polish (music, motion graphics), a pitch deck, a cost model beyond one line.

## Decisions

### D1 · Video structure and timing
Target 4:50 total, hard cap 5:00:
- **0:00–0:05 Cold open:** the NIBIRU shop (sample data label visible) and one line: "One platform where any organiser builds its own ticketing."
- **Act 1, 0:05–1:30, build and publish:** sign up a new organiser; copilot brief → draft event, tiers and themed shop; edit in the builder (brand kit, a section, phone preview); publish; open `/s/<brand>`; the all-in price.
- **Act 2, 1:30–3:25, on-sale:** sale settings (queue on, rate, caps, OTP); start the simulator with 100,000 virtual fans, 20% bots and provider A outage at a set time; the live dashboard (queue, admitted per minute, sold, bots at the back, failover to B); a fan in the queue on a phone frame, admitted, pays from the EPS Wallet in one tap; the sales dashboard and promoter leaderboard moving; cut to `/scale`: measured per-pod numbers and the maths to 100k.
- **Act 3, 3:25–4:20, the fan after purchase:** squad buy for 4, each friend pays their own share from a link; the trip wallet timeline; the rotating QR; the door page scans a screenshot of an old code and rejects it.
- **4:20–4:50 Architecture:** the system sketch (Vercel control plane, Go data plane on DOKS, Neon, Redis Streams), "drafts only" AI, the tests roll-up count, and the URL.

### D2 · Honesty on screen
The narration uses the `/scale` labels: the 100,000 fans are "simulated, through the real services"; throughput is "measured on one instance"; the total is "calculated". The simulator's real fan count on the cluster is stated if it is lower than 100,000. The AI draft is shown as a draft and published by the human.

### D3 · Recording plan
1920×1080 at 30 fps, browser at 100% zoom in a clean profile, EN interface with one RO screen shown (diacritics), phone views via the builder preview and a device frame; screen recorder plus a separate voice track; each act recorded as its own take so a failed live moment costs one act. Backups: a pre-recorded copilot run and simulator run from the rehearsal, used only if the live take fails and then labelled "recorded earlier". Export H.264 MP4, captions file in EN, upload unlisted.

### D4 · Demo reset
One procedure (and a script if needed) run before each take: reset the Neon demo branch to the post-seed snapshot (a dedicated branch, never production data of others), flush the demo organiser's Redis keys, set fake provider knobs to normal, delete simulator Jobs, set the sale opening time to "now + 2 minutes", sign in the three browser profiles (organiser, fan, squad friend), and open the start URLs. The procedure ends with a 2-minute check list (shop loads, queue empty, wallet balance, QR rotating).

### D5 · Vision document
Four sections answering the four questions, about 450 words in English: **Next-gen** (open, white-label builder for every organiser; fair and honest on-sale; fan-first group buying and wallet; AI drafts and agent distribution) and why (the gaps found in the research and in EPS today); **First 6 months** (month-by-month: production payments with two real providers behind the adapter, refunds, onboarding of NIBIRU and Beach, Please! onto one stack, the eMAG channel as a shop, e-money partner for the wallet, security and GDPR review); **100k in 10 minutes** (identity before demand, pre-queue and shuffle, signed passes, rate dial, silent bot scoring, sharded stock, two providers with failover, pre-scaling, static fallback, and the measured numbers with the maths); **Team** (roles and size for the first 6 months). The Romanian version is a faithful translation with correct diacritics, not a summary. Both are exported to PDF to check one page.

## Risks / Trade-offs

- [Live AI or load moment fails during recording] → per-act takes and labelled backups (D3).
- [Over 5 minutes] → timed table rehearsal; cut order decided up front: cold open, then concierge, then the RO screen.
- [Vision over one page] → draft long, then cut; numbers kept to the four that matter.
- [Claims drifting from the measured numbers] → the vision and script cite `/scale`, and a final check compares them.
