// Draft autosave (design D2.4): PUT { revision, document } replaces the draft only at the current
// revision (409 with the current revision otherwise); GET returns the draft for "reload".
import { NextResponse } from "next/server";
import * as z from "zod";
import { DomainError, fromPgError } from "@/lib/errors";
import { assertCanDesign, sameOrigin } from "@/lib/storefront/access";
import { DraftConflictError, getStorefront, saveDraft } from "@/lib/storefront/domain";
import { LIMITS } from "@/lib/storefront/schema";
import { requireActive, studioTx } from "@/lib/studio/context";

const body = z.strictObject({ revision: z.int().min(1), document: z.unknown() });
const noStore = { "Cache-Control": "private, no-store" };

function fail(err: unknown) {
  if (err instanceof DraftConflictError) return NextResponse.json({ code: "draft_conflict", revision: err.currentRevision }, { status: 409, headers: noStore });
  const d = err instanceof DomainError ? err : fromPgError(err);
  if (d?.code === "signed_out" || d?.code === "no_organiser") return NextResponse.json({ code: d.code }, { status: 401, headers: noStore });
  if (d?.code === "not_found") return NextResponse.json({ code: "not_found" }, { status: 404, headers: noStore });
  if (d?.code === "designer_only") return NextResponse.json({ code: d.code }, { status: 403, headers: noStore });
  if (d) return NextResponse.json({ code: d.code, path: d.field ?? "" }, { status: 422, headers: noStore });
  console.error("draft save failed", (err as Error)?.message);
  return NextResponse.json({ code: "unexpected" }, { status: 500, headers: noStore });
}

export async function GET(_req: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await params;
    const s = await requireActive();
    const sf = await studioTx(s, (c) => getStorefront(c, id));
    if (!sf) throw new DomainError("not_found");
    return NextResponse.json({ revision: sf.draft_revision, document: sf.document }, { headers: noStore });
  } catch (err) {
    return fail(err);
  }
}

export async function PUT(req: Request, { params }: { params: Promise<{ id: string }> }) {
  if (!sameOrigin(req)) return NextResponse.json({ code: "forbidden" }, { status: 403, headers: noStore });
  try {
    const { id } = await params;
    const text = await req.text();
    if (text.length > LIMITS.bytes + 1024) return NextResponse.json({ code: "document_too_large", path: "" }, { status: 413, headers: noStore });
    let json: unknown;
    try {
      json = JSON.parse(text);
    } catch {
      return NextResponse.json({ code: "invalid", path: "" }, { status: 400, headers: noStore });
    }
    const p = body.safeParse(json);
    if (!p.success) return NextResponse.json({ code: "invalid", path: "" }, { status: 400, headers: noStore });
    const s = await requireActive();
    assertCanDesign(s.active.role);
    const r = await studioTx(s, (c) => saveDraft(c, id, p.data));
    return NextResponse.json(r, { headers: noStore });
  } catch (err) {
    return fail(err);
  }
}
