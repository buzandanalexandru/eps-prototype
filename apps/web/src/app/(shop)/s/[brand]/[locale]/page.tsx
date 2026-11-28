import { renderShop, shopMetadata } from "../shop";

// Static, generated on first request and kept until the brand's next publish (tag revalidation);
// the hour bounds anything published outside a publish (a newly configured hostname).
export const dynamic = "force-static";
export const revalidate = 3600;
export function generateStaticParams() {
  return [];
}

type P = { params: Promise<{ brand: string; locale: string }> };

export async function generateMetadata({ params }: P) {
  const { brand, locale } = await params;
  return shopMetadata(brand, locale, { kind: "home" });
}

export default async function ShopHome({ params }: P) {
  const { brand, locale } = await params;
  return renderShop(brand, locale, { kind: "home" });
}
