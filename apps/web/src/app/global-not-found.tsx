// The platform 404 for addresses that match no route (two root layouts: platform and shop).
import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = { title: "Not found · EPS", robots: { index: false, follow: false } };

export default function GlobalNotFound() {
  return (
    <html lang="en" className="h-full antialiased">
      <body className="flex min-h-full flex-col items-center justify-center gap-2 bg-background p-8 text-foreground">
        <h1 className="text-2xl font-bold">404</h1>
        <p>This page could not be found. · Pagina nu a fost găsită.</p>
        <a href="/" className="underline">
          EPS
        </a>
      </body>
    </html>
  );
}
