// GET /api/public/events/<id>/availability: the core's public availability answer, served from our
// own origin (design D3.6) so shop pages need no cross-origin request (CSP connect-src 'self', no
// CORS widening, and it works on brand hostnames). The CDN keeps each answer 2 seconds, so an
// on-sale crowd costs about one call to the core per event per 2 seconds per edge region.
import * as z from "zod";
import { coreBaseUrl } from "@/lib/core/url";

export const dynamic = "force-dynamic";

const answer = z.object({
  eventId: z.uuid(),
  version: z.int().min(1),
  generatedAt: z.string().max(40),
  products: z
    .array(
      z.object({
        productId: z.uuid(),
        status: z.enum(["on_sale", "not_yet", "ended", "sold_out"]),
        opensAt: z.string().max(40).optional(),
        priceStepIndex: z.int().min(0).max(100),
        allInMinor: z.int().min(0),
        remaining: z.int().min(0).optional(),
      }),
    )
    .max(500),
});

const problem = (status: number, code: string) =>
  Response.json(
    { type: `https://eps.dev/problems/${code}`, title: code, status, code },
    { status, headers: { "Content-Type": "application/problem+json", "Cache-Control": status === 404 ? "public, max-age=10, s-maxage=10" : "no-store" } },
  );

export async function GET(_req: Request, { params }: { params: Promise<{ eventId: string }> }) {
  const { eventId } = await params;
  if (!z.uuid().safeParse(eventId).success) return problem(404, "not_found");
  const base = coreBaseUrl();
  if (!base) return problem(503, "unavailable");
  let res: Response;
  try {
    res = await fetch(`${base}/v1/public/events/${eventId}/availability`, { cache: "no-store", signal: AbortSignal.timeout(3000), headers: { accept: "application/json" } });
  } catch {
    return problem(503, "unavailable");
  }
  if (res.status === 404) return problem(404, "not_found");
  if (!res.ok) return problem(503, "unavailable");
  const parsed = answer.safeParse(await res.json().catch(() => null));
  if (!parsed.success || parsed.data.eventId !== eventId) return problem(503, "unavailable");
  return Response.json(parsed.data, {
    headers: { "Cache-Control": "public, max-age=2, s-maxage=2", "X-Content-Type-Options": "nosniff" },
  });
}
