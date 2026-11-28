// Shared fixtures for the storefront tests: a valid document for a brand and one event.
import { buildTemplate } from "@/lib/storefront/templates";
import type { StorefrontDocument } from "@/lib/storefront/schema";

export const EVENT_ID = "11111111-1111-4111-8111-111111111111";
export const EVENT = { id: EVENT_ID, slug: "galaxia-2027", name: { ro: "Galaxia 2027", en: "Galaxia 2027" } };

export function doc(template: "festival" | "resort" | "club" | "theatre" = "festival", event: typeof EVENT | null = EVENT): StorefrontDocument {
  return buildTemplate(template, { brandName: "Test Brand", languages: ["ro", "en"], event });
}

export const clone = <T>(v: T): T => structuredClone(v);

import { ensureStorefront } from "@/lib/storefront/domain";
import { festival } from "./catalog-fixtures";
import { makeOrganiser, tx } from "./db";

/** An organiser whose brand has one event (with a 539.00 RON pass) and a storefront from the festival template. */
export async function shopOrganiser() {
  const o = await makeOrganiser();
  const f = await tx(o.scope, (c) => festival(c, o.brandId));
  const sf = await tx(o.scope, (c) => ensureStorefront(c, o.brandId, o.user.id));
  return { ...o, f, sf };
}
