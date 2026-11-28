// Storefront spec: all-in prices everywhere, "from" prices, honest ladder notes and instalment
// summaries, all from the snapshot's precomputed amounts (design D3.7).
import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { ESLint } from "eslint";
import { feeBreakdown } from "@/lib/pricing/fees";
import { shopEventFromSnapshot } from "@/lib/storefront/shop-view";
import { fromPriceAt, instalmentsAt, priceAt } from "@/lib/storefront/sale-view";
import { SHOP_LABELS } from "@/lib/storefront/labels";
import type { SaleSnapshot, SnapshotPriceStep } from "@/lib/snapshot/types";
import { AllInPrice, PriceNotes } from "@/components/storefront/all-in-price";
import { galaxiaSnapshot, GA, CAMPING } from "./snapshot-fixture";

const text = (html: string) => html.replace(/<[^>]+>/g, " ").replace(/&amp;/g, "&").replace(/\s+/g, " ").trim();
const DEC5 = new Date("2026-12-05T10:00:00Z");

function step(index: number, faceMinor: number, policy: "absorb" | "pass_on", fromAt: string | null = null): SnapshotPriceStep {
  const b = feeBreakdown(faceMinor, policy, { percentBps: 250, fixedMinor: 0 });
  return { index, mode: "by_date", fromAt, afterSold: null, faceMinor, allInMinor: b.allInMinor, fanFeeMinor: b.fanFeeMinor, organiserNetMinor: b.organiserNetMinor };
}

/** A ticket card's price block as the browser shows it at `now` (the page itself may be older). */
function card(snap: SaleSnapshot, productId: string, locale: "ro" | "en", now = DEC5) {
  const ev = shopEventFromSnapshot(snap, null, new Date("2026-10-01T00:00:00Z")); // a page cached since October
  const p = ev.products.find((x) => x.id === productId)!;
  const price = priceAt(p, now);
  return text(
    renderToStaticMarkup(
      <>
        <AllInPrice allInMinor={price.allInMinor} fanFeeMinor={price.fanFeeMinor} currency={ev.currency} locale={locale} labels={SHOP_LABELS[locale]} showFee />
        <PriceNotes
          sale={p.sale!}
          stepIndex={price.stepIndex}
          instalments={instalmentsAt(p.sale!, price.allInMinor, now, ev.startsAt, ev.timezone)}
          currency={ev.currency}
          locale={locale}
          labels={SHOP_LABELS[locale]}
          timeZone={ev.timezone}
        />
      </>,
    ),
  );
}

describe("all-in prices", () => {
  it("pass on: GA shows RON 552.48 (552,48 RON) and the fee it includes", () => {
    const snap = galaxiaSnapshot();
    expect(card(snap, GA, "en")).toContain("RON 552.48 final price, fees included · includes platform fee of RON 13.48");
    expect(card(snap, GA, "ro")).toContain("552,48 RON preț final, taxe incluse · include taxa platformei de 13,48 RON");
  });

  it("absorb: the face price is the all-in price, with no fee line", () => {
    const snap = galaxiaSnapshot();
    snap.fee.policy = "absorb";
    snap.products[0].priceSteps = [step(0, 53900, "absorb")];
    const t = card(snap, GA, "en");
    expect(t).toContain("RON 539.00");
    expect(t).not.toContain("platform fee");
  });

  it('"from" is the lowest visible ticket on sale, ignoring hidden products and add-ons', () => {
    const snap = galaxiaSnapshot();
    const base = snap.products[0];
    const mk = (id: string, face: number, visible = true) => ({ ...base, productId: id, visible, priceSteps: [step(0, face, "pass_on")], instalments: null });
    snap.products = [
      mk("0d000000-0000-4000-8000-0000000000a1", 53900),
      mk("0d000000-0000-4000-8000-0000000000a2", 65900),
      mk("0d000000-0000-4000-8000-0000000000a3", 83900),
      mk("0d000000-0000-4000-8000-0000000000a4", 9756, false), // 100.00 all-in, hidden
      { ...snap.products[1], priceSteps: [step(0, 1000, "pass_on")] }, // an add-on
    ];
    const ev = shopEventFromSnapshot(snap, null, DEC5);
    expect(ev.products.map((p) => p.allInMinor)).toEqual([55248, 67548, 85998, 1025]);
    // An add-on requiring a hidden product publishes no hidden id.
    snap.products[4].requires = ["0d000000-0000-4000-8000-0000000000a4", "0d000000-0000-4000-8000-0000000000a1"];
    expect(shopEventFromSnapshot(snap, null, DEC5).products.at(-1)!.sale!.requires).toEqual(["0d000000-0000-4000-8000-0000000000a1"]);
    expect(fromPriceAt(ev, DEC5)).toEqual({ allInMinor: 55248, upcoming: null });
    // Nothing on sale yet: the lowest upcoming price, labelled with its opening.
    for (const p of snap.products) p.salesFrom = "2027-02-01T08:00:00.000Z";
    expect(fromPriceAt(shopEventFromSnapshot(snap, null, DEC5), DEC5)).toEqual({ allInMinor: 55248, upcoming: "2027-02-01T08:00:00.000Z" });
    // The page does not carry hidden products at all.
    expect(ev.products.some((p) => p.id.endsWith("a4"))).toBe(false);
  });
});

describe("ladder notes", () => {
  it("Promenada on 15 Dec 2026: the 11 RON step, rising to 13 RON on 1 Jan 2027, with the platform note", () => {
    const snap = galaxiaSnapshot();
    snap.products[0] = {
      ...snap.products[0],
      instalments: null,
      priceSteps: [step(0, 1000, "pass_on"), step(1, 1100, "pass_on", "2026-11-30T22:00:00.000Z"), step(2, 1300, "pass_on", "2026-12-31T22:00:00.000Z")],
    };
    const t = card(snap, GA, "en", new Date("2026-12-15T10:00:00Z"));
    expect(t).toContain("RON 11.28");
    expect(t).toContain("Price rises to RON 13.33 on 1 Jan 2027");
    expect(t).toContain("Prices depend only on the date or on tickets sold, never on who is buying.");
    expect(card(snap, GA, "ro", new Date("2026-12-15T10:00:00Z"))).toContain("Prețul crește la 13,33 RON pe 1 ian. 2027");
  });

  it("a sales ladder never shows its threshold", () => {
    const t = card(galaxiaSnapshot(), GA, "en");
    expect(t).toContain("Price may rise as tickets sell");
    expect(t).not.toContain("10,000");
    expect(t).not.toContain("614"); // the next step's price
  });
});

describe("instalments", () => {
  it("GA viewed on 5 Dec 2026: RON 10.00 now + 7 × RON 77.49, the last RON 77.54, totalling RON 552.48", () => {
    const t = card(galaxiaSnapshot(), GA, "en");
    expect(t).toContain("RON 10.00 now + 7 × RON 77.49");
    expect(t).toContain("Last payment RON 77.54. Total RON 552.48.");
    expect(1000 + 6 * 7749 + 7754).toBe(55248);
    expect(card(galaxiaSnapshot(), CAMPING, "en")).not.toContain("now +");
  });
});

describe("money formatting rule", () => {
  it("storefront code may not format money outside the all-in price component", async () => {
    const eslint = new ESLint({ cwd: process.cwd() });
    const [r] = await eslint.lintText('import { formatMoney } from "@/lib/pricing/money";\nexport const x = formatMoney(1, "RON", "en");\n', { filePath: "src/components/storefront/new-card.tsx" });
    expect(r.messages.map((m) => m.ruleId)).toContain("no-restricted-imports");
    const [n] = await eslint.lintText('export const y = new Intl.NumberFormat("en").format(1);\n', { filePath: "src/app/(shop)/s/[brand]/x.tsx" });
    expect(n.messages.map((m) => m.ruleId)).toContain("no-restricted-syntax");
  }, 60_000);
});
