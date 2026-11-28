import { cookies, headers } from "next/headers";
import { getRequestConfig } from "next-intl/server";
import { isLocale, LOCALE_COOKIE, resolveLocale } from "./locale";

export default getRequestConfig(async () => {
  const h = await headers();
  // Shop pages carry their language in the URL (/s/<brand>/<locale>); the proxy passes it on.
  const shop = h.get("x-eps-shop-locale");
  const locale = isLocale(shop) ? shop : resolveLocale((await cookies()).get(LOCALE_COOKIE)?.value, h.get("accept-language"));
  return {
    locale,
    messages: (await import(`../../messages/${locale}.json`)).default,
  };
});
