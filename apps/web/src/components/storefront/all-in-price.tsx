// Every fan-facing amount in the shop goes through this module (design D3.7): the all-in price from
// a snapshot price step, the fee line, ladder notes and instalment summaries. A lint rule refuses
// money formatting anywhere else in storefront code, so no screen can show a price without its fee.
import { formatMoney, type Currency } from "@/lib/pricing/money";
import type { ShopSaleProduct } from "@/lib/storefront/catalogue";
import { fill, type ShopLabels } from "@/lib/storefront/labels";
import { nextDateStep, type instalmentsAt } from "@/lib/storefront/sale-view";
import type { Locale } from "@/lib/storefront/schema";

/** An amount as the fan reads it: "552,48 RON" in Romanian, "RON 552.48" in English. */
export function money(minor: number, currency: Currency, locale: Locale): string {
  return formatMoney(minor, currency, locale);
}

const intl = (l: Locale) => (l === "ro" ? "ro-RO" : "en-GB");

/** A date in the event's time zone, e.g. "1 Jan 2027" / "1 ian. 2027". */
export function shopDate(iso: string, locale: Locale, timeZone: string): string {
  return new Intl.DateTimeFormat(intl(locale), { timeZone, day: "numeric", month: "short", year: "numeric" }).format(new Date(iso));
}

export function AllInPrice(props: {
  allInMinor: number;
  fanFeeMinor: number;
  currency: Currency;
  locale: Locale;
  labels: ShopLabels;
  /** Product details: add "includes platform fee of X" when the fan pays a fee share. */
  showFee?: boolean;
  className?: string;
}) {
  return (
    <div className="min-w-0">
      <p className={props.className ?? "text-2xl font-bold text-[var(--eps-primary)]"} data-testid="ticket-price">
        {money(props.allInMinor, props.currency, props.locale)}
      </p>
      <p className="text-xs text-[var(--eps-muted)]">
        {props.labels.allIn}
        {props.showFee && props.fanFeeMinor > 0 && (
          <>
            {" · "}
            <span data-testid="fee-line">{fill(props.labels.includesFeeOf, { fee: money(props.fanFeeMinor, props.currency, props.locale) })}</span>
          </>
        )}
      </p>
    </div>
  );
}

/** Ladder notes and the instalment summary of a product at a moment, from its snapshot price steps. */
export function PriceNotes({ sale, stepIndex, instalments, currency, locale, labels, timeZone }: {
  sale: ShopSaleProduct;
  stepIndex: number;
  instalments: ReturnType<typeof instalmentsAt>;
  currency: Currency;
  locale: Locale;
  labels: ShopLabels;
  timeZone: string;
}) {
  const ladder = sale.steps.length > 1;
  const next = nextDateStep(sale, stepIndex);
  const mode = sale.steps[stepIndex]?.mode;
  const i = instalments;
  return (
    <div className="space-y-1 text-xs text-[var(--eps-muted)]">
      {ladder && mode === "by_date" && next && (
        <p data-testid="ladder-next">{fill(labels.priceRises, { price: money(next.allInMinor, currency, locale), date: shopDate(next.fromAt, locale, timeZone) })}</p>
      )}
      {ladder && mode === "by_sales" && <p data-testid="ladder-sales">{labels.mayRise}</p>}
      {ladder && <p data-testid="ladder-note">{labels.ladderNote}</p>}
      {i && (
        <div data-testid="instalments">
          <p className="font-semibold text-[var(--eps-on-surface)]">
            {fill(labels.instalments, { deposit: money(i.depositMinor, currency, locale), count: i.count, each: money(i.eachMinor, currency, locale) })}
          </p>
          <p>{fill(labels.instalmentsLast, { last: money(i.lastMinor, currency, locale), total: money(i.totalMinor, currency, locale) })}</p>
        </div>
      )}
    </div>
  );
}

/** A plain amount inside storefront text (totals), formatted by the same rule. */
export function Money({ minor, currency, locale, testId }: { minor: number; currency: Currency; locale: Locale; testId?: string }) {
  return <span data-testid={testId}>{money(minor, currency, locale)}</span>;
}
