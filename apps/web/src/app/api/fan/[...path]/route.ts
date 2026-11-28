// /api/fan/<path>: the trip wallet, live ticket, signed guest link and ticket keys (step 7),
// forwarded to the core's /v1/<path> (lib/fan/routes.ts lists what passes). Never cached.
import { forwardToCore } from "@/lib/fan/forward";
import { FAN_ROUTES, matchRoute } from "@/lib/fan/routes";

export const dynamic = "force-dynamic";

type P = { params: Promise<{ path: string[] }> };

async function handle(req: Request, { params }: P) {
  const path = (await params).path.join("/");
  return forwardToCore(req, matchRoute(FAN_ROUTES, req.method, path), `/v1/${path}`);
}

export const GET = handle;
export const POST = handle;
export const PUT = handle;
