import { renderShop, shopMetadata } from "../../../shop";

export const dynamic = "force-dynamic";

type P = { params: Promise<{ brand: string; locale: string; event: string }> };

export async function generateMetadata({ params }: P) {
  const { brand, locale, event } = await params;
  return shopMetadata(brand, locale, { kind: "event", slug: event });
}

export default async function ShopEventPage({ params }: P) {
  const { brand, locale, event } = await params;
  return renderShop(brand, locale, { kind: "event", slug: event });
}
