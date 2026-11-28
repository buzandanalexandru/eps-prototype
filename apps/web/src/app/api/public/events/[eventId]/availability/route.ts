// GET /api/public/events/<id>/availability: the core's public availability answer, served from our
// own origin (design D3.6) so shop pages need no cross-origin request (CSP connect-src 'self', no
// CORS widening, and it works on brand hostnames). The CDN keeps each answer 2 seconds (and serves
// it a little longer while refreshing or when the core fails), so an on-sale crowd costs about one
// call to the core per event per 2 seconds per edge region. A query string is refused, so nobody
// can bypass the CDN's copy by varying it; failures are cached 5 seconds, a 404 never publicly.
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
        allHeld: z.boolean().optional(),
      }),
    )
    .max(500),
  instances: z
    .array(z.object({ instanceId: z.string().regex(/^[0-9a-f-]{36}(@\d{4}-\d{2}-\d{2}(T\d{2}:\d{2})?)?$/), status: z.enum(["on_sale", "sold_out"]), remaining: z.int().min(0), allHeld: z.boolean().optional() }))
    .max(20_000)
    .default([]),
});

/** 503: cached 5 s at the edge so a failing core is not hammered; 400 and 404: never cached publicly. */
const CACHE: Record<number, string> = { 503: "public, max-age=5, s-maxage=5" };

const problem = (status: number, code: string) =>
  Response.json(
    { type: `https://eps.dev/problems/${code}`, title: code, status, code },
    { status, headers: { "Content-Type": "application/problem+json", "Cache-Control": CACHE[status] ?? "no-store" } },
  );

export async function GET(req: Request, { params }: { params: Promise<{ eventId: string }> }) {
  if (new URL(req.url).search) return problem(400, "invalid_request");
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
    headers: { "Cache-Control": "public, max-age=2, s-maxage=2, stale-while-revalidate=10, stale-if-error=60", "X-Content-Type-Options": "nosniff" },
  });
}
