// The locked checkout and the order status page (step 4). Static like every shop page: the shop's
// chrome is rendered once per publish and the checkout island reads the address in the browser
// (a hand-off ?event=&v=&items= to hold, or ?order=<id>[&t=<link>] to show). Nothing in the URL is
// trusted: the core re-prices from snapshot `v` and re-applies every rule.
import { Checkout } from "@/components/storefront/checkout";
import { SHOP_LABELS } from "@/lib/storefront/labels";
import { renderShop, shopMetadata } from "../../shop";

export const dynamic = "force-static";
export const revalidate = 3600;
export function generateStaticParams() {
  return [];
}

type P = { params: Promise<{ brand: string; locale: string }> };

export async function generateMetadata({ params }: P) {
  const { brand, locale } = await params;
  return shopMetadata(brand, locale, { kind: "checkout" });
}

export default async function ShopCheckout({ params }: P) {
  const { brand, locale } = await params;
  return renderShop(brand, locale, { kind: "checkout" }, ({ locale: loc, base }) => <Checkout labels={SHOP_LABELS[loc]} locale={loc} base={base} />);
}
