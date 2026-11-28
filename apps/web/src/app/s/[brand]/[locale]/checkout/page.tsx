// The checkout route (spec map): step 4 builds checkout here. Until then it shows the selection the
// picker handed over, priced from the live snapshot, and "Checkout opens soon".
import { Money } from "@/components/storefront/all-in-price";
import { SHOP_LABELS } from "@/lib/storefront/labels";
import { parseItems } from "@/lib/storefront/picker";
import { renderShop, shopMetadata } from "../../shop";

export const dynamic = "force-dynamic";

type P = { params: Promise<{ brand: string; locale: string }>; searchParams: Promise<{ event?: string; v?: string; items?: string }> };

export async function generateMetadata({ params }: P) {
  const { brand, locale } = await params;
  return shopMetadata(brand, locale, { kind: "checkout" });
}

export default async function ShopCheckout({ params, searchParams }: P) {
  const { brand, locale } = await params;
  const sp = await searchParams;
  return renderShop(brand, locale, { kind: "checkout" }, ({ cat, locale: loc, base }) => {
    const labels = SHOP_LABELS[loc];
    const ev = typeof sp.event === "string" ? cat[sp.event] : undefined;
    const lines = (parseItems(typeof sp.items === "string" ? sp.items : null) ?? [])
      .map((l) => ({ ...l, product: ev?.products.find((p) => p.id === l.productId) }))
      .filter((l) => !!l.product);
    const total = lines.reduce((s, l) => s + l.qty * l.product!.allInMinor, 0);
    return (
      <div className="mx-auto max-w-3xl space-y-4 px-4 py-10" data-testid="checkout-placeholder" data-event={ev?.id ?? ""} data-version={sp.v ?? ""}>
        <h1 className="font-[family-name:var(--eps-font-heading)] text-3xl font-bold">{labels.checkoutTitle}</h1>
        <p className="font-semibold">{labels.checkoutSoon}</p>
        {ev && lines.length > 0 ? (
          <>
            <p className="text-sm">{labels.checkoutSoonLead}</p>
            <ul className="space-y-1" data-testid="checkout-lines">
              {lines.map((l, i) => (
                <li key={i} className="flex flex-wrap justify-between gap-2" data-testid="checkout-line">
                  <span>
                    {l.qty} × {l.product!.name[loc] || l.product!.name.ro}
                    {l.instance ? ` · ${l.instance.replace("T", " ")}` : ""}
                  </span>
                  <Money minor={l.qty * l.product!.allInMinor} currency={ev.currency} locale={loc} />
                </li>
              ))}
            </ul>
            <p className="text-lg font-bold">
              {labels.total}: <Money minor={total} currency={ev.currency} locale={loc} testId="checkout-total" />
            </p>
          </>
        ) : (
          <p className="text-sm">{labels.nothingSelected}</p>
        )}
        <a href={base} className="text-sm font-semibold underline">
          {labels.backToShop}
        </a>
      </div>
    );
  });
}
