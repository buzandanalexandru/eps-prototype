import { renderShop, shopMetadata } from "../shop";

export const dynamic = "force-dynamic";

type P = { params: Promise<{ brand: string; locale: string }> };

export async function generateMetadata({ params }: P) {
  const { brand, locale } = await params;
  return shopMetadata(brand, locale, { kind: "home" });
}

export default async function ShopHome({ params }: P) {
  const { brand, locale } = await params;
  return renderShop(brand, locale, { kind: "home" });
}
