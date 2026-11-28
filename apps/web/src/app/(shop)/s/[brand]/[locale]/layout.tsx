// The public shop's own root layout (design D3.6): no cookies, no headers and no platform
// translations, so shop pages render statically, are cached at the edge until the brand's next
// publish, and ship only the shop's own small client islands.
import type { Viewport } from "next";
import "../../../../globals.css";

export const viewport: Viewport = { width: "device-width", initialScale: 1 };

export default async function ShopRootLayout({ children, params }: { children: React.ReactNode; params: Promise<{ locale: string }> }) {
  const { locale } = await params;
  return (
    <html lang={locale === "en" ? "en" : "ro"} className="h-full antialiased">
      <body className="min-h-full">{children}</body>
    </html>
  );
}
