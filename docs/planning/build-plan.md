# Build plan

The build steps agreed in the planning session on 27 Nov 2026. Each step becomes one OpenSpec change (`openspec/changes/<name>/`), written before the 48-hour clock starts. All code is written inside the 48 hours.
Feature IDs refer to `feature-scope.md`; decision IDs (S, P, O, U, R) refer to `architecture.md`.

| # | Change | Builds | Unlocks |
|---|---|---|---|
| 0 ✅ | `step-0-foundation` (archived) | Monorepo, Next.js app, Go module, OpenAPI contract, Postgres schemas + row-level security, sign-in, Helm skeleton, CI, hello-world on Vercel and DOKS (S1–S3, P2, U1, U2, U5, I1, I5) | Everything |
| 1 ✅ | `step-1-organisers-and-catalogue` (archived) | Sign-up, brands, roles, fees, events, product types, pools, price ladder, rules, instalment setting, NIBIRU seed (A1–A3, A6, C1–C5, C8–C10) | Builder, sales |
| 2 ✅ | `step-2-builder` (archived) | Brand kit, templates, sections, live preview, draft/publish/rollback, locked checkout, languages, email/ticket design (B1–B7, B10, B12, U3) | Act 1 |
| 3 ✅ | `step-3-publish-and-storefront` (archived) | Sale snapshot, `/s/<brand>` storefronts, all-in price (P1, D1) | Selling |
| 4 ✅ | `step-4-checkout-and-payments` (archived) | Holds, order states, fake providers A/B, failover, idempotency, resume link, guest checkout, questions, instalments, Apple/Google Pay, wallet (D2–D9, D11–D13, O3, O4, O6, P3) | Act 2 checkout |
| 5 ✅ | `step-5-onsale-engine` (archived) | Sale settings, waiting room and shuffle, signed passes, rate dial, bot scoring, caps, OTP (E1–E6, E12, O1, O2) | Act 2 queue |
| 6 | `step-6-simulator-and-dashboard` | Simulator, live dashboard, metrics, graceful degradation (E7, E8, E11, I4, R2, R3) | Act 2 complete |
| 7 | `step-7-tickets-and-wallet` | Trip wallet, rotating QR, personalise and check in, fan account, door scanning, wallet pass preview (F2–F5, F8, G2, O5) | Act 3 |
| 8 | `step-8-squad-buy` | Group hold, each friend pays their own share (F1, O7) | Act 3 complete |
| 9 | `step-9-resale-waitlist-lottery` | Face-value resale, self-filling waitlist, lottery, upgrades (F6, E9, E10, C7) | Extras |
| 10 | `step-10-organiser-growth` | Sales dashboard, audience export, webhooks, Influencer & Promoter Hub, extra shops, marketplace, payment accounts (G1, G3, G5, C6, B11, A4, A5) | Breadth |
| 11 | `step-11-ai` | Organiser copilot, fan concierge, MCP server (H1–H3, U4) | Next-gen moment |
| 12 | `step-12-scale-proof` | Final Helm and autoscaling, k6 numbers, roadmap page (I2, I3, R1, R4 roll-up, G6 as vision) | The 100k answer |
| 13 | `step-13-deliverables` | 5-minute video script and recording plan, one-page vision | Submission |

## Cut lines
- **After step 6:** Acts 1 and 2 work.
- **After step 8:** the full three-act demo works.
- **Steps 9–11** are extras, in the order chosen.
- **Step 12** is protected: it carries the 100k answer.
