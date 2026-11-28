// Renders a storefront page to HTML for the renderer tests, with a fixed catalogue.
import { renderToStaticMarkup } from "react-dom/server";
import { StorefrontView } from "@/components/storefront/storefront";
import { SHOP_LABELS } from "@/lib/storefront/labels";
import { doc, EVENT, EVENT_ID } from "./storefront-fixtures";

export const catalogue = {
  [EVENT_ID]: {
    id: EVENT_ID,
    slug: EVENT.slug,
    name: EVENT.name,
    startsAt: "2027-07-15T14:00:00.000Z",
    endsAt: "2027-07-19T00:00:00.000Z",
    timezone: "Europe/Bucharest",
    currency: "RON" as const,
    venue: { name: "Arena", address: "Costinești", lat: 43.93, lng: 28.63, map: "/assets/maps/15/18862/11919.png" },
    days: [{ date: "2027-07-15", opens: "17:00", closes: "03:00" }],
    lineup: [{ date: "2027-07-15", artist: "Headliner", stage: "Main", starts: "22:00" }],
    products: [{ id: "p1", type: "pass", name: { ro: "Abonament GA", en: "GA pass" }, perks: [{ ro: "Acces", en: "Entry" }], allInMinor: 55248, fanFeeMinor: 1348 }],
  },
};

export function render(d = doc(), locale: "ro" | "en" = "en", pageIndex = 0, mode: "preview" | "link" | "published" = "preview") {
  return renderToStaticMarkup(
    <StorefrontView
      doc={d}
      page={d.pages[pageIndex]}
      ctx={{ locale, labels: SHOP_LABELS[locale], catalogue, brand: { name: "Brand", slug: "brand", currency: "RON", organiserName: "Org SRL", isSample: true }, mode, hrefBase: "/s/brand/en" }}
    />,
  );
}

