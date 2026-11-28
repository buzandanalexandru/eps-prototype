// The shop's EPS Wallet page (step 4, D12): the signed-in fan's simulated balance at this shop's
// organiser, top-ups through the simulated providers, the wallet code for stands and the history.
// Static like every shop page; the island asks the core in the browser.
import { WalletView } from "@/components/storefront/wallet";
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
  return shopMetadata(brand, locale, { kind: "wallet" });
}

export default async function ShopWallet({ params }: P) {
  const { brand, locale } = await params;
  return renderShop(brand, locale, { kind: "wallet" }, ({ shop, locale: loc, base }) => (
    <WalletView labels={SHOP_LABELS[loc]} locale={loc} base={base} organiserId={shop.organiserId} currency={shop.currency} />
  ));
}
