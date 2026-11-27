"use server";

import { cookies, headers } from "next/headers";
import { redirect } from "next/navigation";
import { isLocale, LOCALE_COOKIE } from "@/i18n/locale";
import { getAuth } from "@/lib/auth/server";

export async function setLocale(form: FormData) {
  const locale = String(form.get("locale"));
  if (isLocale(locale)) {
    (await cookies()).set(LOCALE_COOKIE, locale, { path: "/", maxAge: 365 * 24 * 3600, sameSite: "lax" });
  }
  const back = (await headers()).get("referer");
  let path = "/";
  try {
    if (back) path = new URL(back).pathname + new URL(back).search;
  } catch {}
  redirect(path.startsWith("/") && !path.startsWith("//") ? path : "/");
}

export async function signOut() {
  await getAuth().api.signOut({ headers: await headers() });
  redirect("/sign-in");
}
