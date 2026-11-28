import { renderShop, shopMetadata } from "../../shop";

export const dynamic = "force-dynamic";

type P = { params: Promise<{ brand: string; locale: string; page: string }> };

export async function generateMetadata({ params }: P) {
  const { brand, locale, page } = await params;
  return shopMetadata(brand, locale, { kind: "info", slug: page });
}

export default async function ShopInfoPage({ params }: P) {
  const { brand, locale, page } = await params;
  return renderShop(brand, locale, { kind: "info", slug: page });
}
