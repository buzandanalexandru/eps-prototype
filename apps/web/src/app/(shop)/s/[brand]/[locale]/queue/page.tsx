// The waiting room (step 5, E2-E4): a static page like every shop page. Its island joins the sale's
// line through our own origin (/api/shop/queue/..., forwarded to the gateway), shows the pre-queue
// countdown, the fans ahead and the estimate, and once admitted keeps the signed pass and goes on
// to the checkout (?sale=<id>&next=<checkout address>).
import { WaitingRoom } from "@/components/storefront/waiting-room";
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
  return shopMetadata(brand, locale, { kind: "queue" });
}

export default async function ShopQueue({ params }: P) {
  const { brand, locale } = await params;
  return renderShop(brand, locale, { kind: "queue" }, ({ locale: loc, base }) => <WaitingRoom labels={SHOP_LABELS[loc]} locale={loc} base={base} />);
}
