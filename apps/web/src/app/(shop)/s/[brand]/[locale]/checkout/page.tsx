// The checkout route (spec map): step 4 builds checkout here. Until then it shows the selection the
// picker handed over and "Checkout opens soon". Nothing in the URL is trusted: only the event's
// visible products, valid days and slots, merged lines and every per-order and per-account limit
// (normaliseSelection). Step 4 re-prices everything in the core from snapshot `v`.
import { Money, shopDate } from "@/components/storefront/all-in-price";
import { SHOP_LABELS } from "@/lib/storefront/labels";
import { normaliseSelection, parseItems } from "@/lib/storefront/picker";
import { priceAt } from "@/lib/storefront/sale-view";
import { renderShop, shopMetadata } from "../../shop";

export const dynamic = "force-dynamic";

type P = { params: Promise<{ brand: string; locale: string }>; searchParams: Promise<Record<string, string | string[] | undefined>> };

export async function generateMetadata({ params }: P) {
  const { brand, locale } = await params;
  return shopMetadata(brand, locale, { kind: "checkout" });
}

const one = (v: string | string[] | undefined) => (typeof v === "string" ? v : undefined);

export default async function ShopCheckout({ params, searchParams }: P) {
  const { brand, locale } = await params;
  const sp = await searchParams;
  const event = one(sp.event) ?? "";
  const v = one(sp.v) ?? "";
  const items = one(sp.items) ?? "";
  const query = `?${new URLSearchParams({ event, v, items }).toString()}`;
  return renderShop(
    brand,
    locale,
    { kind: "checkout" },
    ({ cat, locale: loc, base, now }) => {
      const labels = SHOP_LABELS[loc];
      const ev = Object.hasOwn(cat, event) ? cat[event] : undefined;
      const versionOk = !!ev && /^\d{1,7}$/.test(v) && Number(v) === ev.sale?.version;
      const lines = ev && versionOk ? normaliseSelection(ev, parseItems(items) ?? []) : [];
      const priced = lines.map((l) => ({ ...l, product: ev!.products.find((p) => p.id === l.productId)! }));
      const total = priced.reduce((s, l) => s + l.qty * priceAt(l.product, now).allInMinor, 0);
      const when = (instance: string) => {
        const [d, t] = instance.split("T");
        return `${shopDate(`${d}T12:00:00Z`, loc, "UTC")}${t ? `, ${t}` : ""}`;
      };
      return (
        <div className="mx-auto max-w-3xl space-y-4 px-4 py-10" data-testid="checkout-placeholder" data-event={ev?.id ?? ""} data-version={versionOk ? v : ""}>
          <h1 className="font-[family-name:var(--eps-font-heading)] text-3xl font-bold">{labels.checkoutTitle}</h1>
          <p className="font-semibold">{labels.checkoutSoon}</p>
          {ev && priced.length > 0 ? (
            <>
              <p className="text-sm">{labels.checkoutSoonLead}</p>
              <ul className="space-y-1" data-testid="checkout-lines">
                {priced.map((l, i) => (
                  <li key={i} className="flex flex-wrap justify-between gap-2" data-testid="checkout-line">
                    <span>
                      {l.qty} × {l.product.name[loc] || l.product.name.ro}
                      {l.instance ? ` · ${when(l.instance)}` : ""}
                    </span>
                    <Money minor={l.qty * priceAt(l.product, now).allInMinor} currency={ev.currency} locale={loc} />
                  </li>
                ))}
              </ul>
              <p className="text-lg font-bold">
                {labels.total}: <Money minor={total} currency={ev.currency} locale={loc} testId="checkout-total" />
              </p>
            </>
          ) : (
            <p className="text-sm" data-testid="checkout-empty">
              {labels.nothingSelected}
            </p>
          )}
          <a href={base} className="text-sm font-semibold underline">
            {labels.backToShop}
          </a>
        </div>
      );
    },
    query,
  );
}
