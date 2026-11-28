import { renderShop, shopMetadata } from "../../shop";

// Static, generated on first request and kept until the brand's next publish (tag revalidation);
// the hour bounds anything published outside a publish (a newly configured hostname).
export const dynamic = "force-static";
export const revalidate = 3600;
export function generateStaticParams() {
  return [];
}

type P = { params: Promise<{ brand: string; locale: string; page: string }> };

export async function generateMetadata({ params }: P) {
  const { brand, locale, page } = await params;
  return shopMetadata(brand, locale, { kind: "info", slug: page });
}

export default async function ShopInfoPage({ params }: P) {
  const { brand, locale, page } = await params;
  return renderShop(brand, locale, { kind: "info", slug: page });
}
