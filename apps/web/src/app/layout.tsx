import type { Metadata, Viewport } from "next";
import { NextIntlClientProvider } from "next-intl";
import { getLocale, getTranslations } from "next-intl/server";
import { headers } from "next/headers";
import Link from "next/link";
import { LocaleSwitcher } from "@/components/locale-switcher";
import "./globals.css";

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("common");
  return { title: { default: "EPS", template: "%s · EPS" }, description: t("tagline") };
}

export const viewport: Viewport = { width: "device-width", initialScale: 1 };

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const locale = await getLocale();
  const t = await getTranslations("common");
  const layout = (await headers()).get("x-eps-layout");
  if (layout === "bare") {
    return (
      <html lang={locale} className="h-full antialiased">
        <body className="min-h-full">
          <NextIntlClientProvider>{children}</NextIntlClientProvider>
        </body>
      </html>
    );
  }
  return (
    <html lang={locale} className="h-full antialiased">
      <body className="flex min-h-full flex-col bg-background text-foreground">
        <NextIntlClientProvider>
          <header className="border-b">
            <nav className="mx-auto flex max-w-4xl flex-wrap items-center justify-between gap-3 px-4 py-3">
              <Link href="/" className="font-semibold">
                {t("appName")}
              </Link>
              <div className="flex flex-wrap items-center gap-4 text-sm">
                <Link href="/studio">{t("studio")}</Link>
                <Link href="/fees">{t("fees")}</Link>
                <Link href="/status">{t("status")}</Link>
                <LocaleSwitcher current={locale} label={t("language")} />
              </div>
            </nav>
          </header>
          <main className={layout === "wide" ? "w-full flex-1 px-3 py-4 sm:px-4" : "mx-auto w-full max-w-4xl flex-1 px-4 py-8"}>{children}</main>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
