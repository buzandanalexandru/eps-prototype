// /api/door/<path>: the door page's device registration, sync and scan uploads (step 7), forwarded
// to the core's /v1/organiser/<path> with the signed-in organiser's token. Never cached.
import { forwardToCore } from "@/lib/fan/forward";
import { DOOR_ROUTES, doorQuery, matchRoute } from "@/lib/fan/routes";

export const dynamic = "force-dynamic";

type P = { params: Promise<{ path: string[] }> };

async function handle(req: Request, { params }: P) {
  const path = (await params).path.join("/");
  const q = req.method === "GET" ? doorQuery(new URL(req.url).searchParams) : "";
  return forwardToCore(req, matchRoute(DOOR_ROUTES, req.method, path), `/v1/organiser/${path}${q}`, { organiser: true });
}

export const GET = handle;
export const POST = handle;
