// /s/<brand>: redirects (307) to the shop in the visitor's language: the language cookie, then the
// browser's languages, then the shop's default, among the languages the shop offers (D3.5).
import { cookies, headers } from "next/headers";
import { notFound, redirect } from "next/navigation";
import { LOCALE_COOKIE } from "@/i18n/locale";
import { pickShopLocale, shopBase } from "@/lib/storefront/routing";
import { shopFor } from "./shop";

export const dynamic = "force-dynamic";

export default async function ShopRoot({ params }: { params: Promise<{ brand: string }> }) {
  const { brand } = await params;
  const found = await shopFor(brand);
  if (!found) notFound();
  const { shop, key } = found;
  const locale = pickShopLocale({
    cookie: (await cookies()).get(LOCALE_COOKIE)?.value,
    acceptLanguage: (await headers()).get("accept-language"),
    enabled: shop.document.locales.enabled,
    fallback: shop.document.locales.default,
  });
  redirect(shopBase(key, locale));
}
