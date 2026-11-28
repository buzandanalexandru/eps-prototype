// Storefront builder spec "Private preview links": hashed tokens, expiry, revocation, guessing.
import { randomBytes } from "node:crypto";
import { expect, it } from "vitest";
import { createPreviewLink, hashPreviewToken, listPreviewLinks, resolvePreviewToken, revokePreviewLink } from "@/lib/storefront/domain";
import { layoutFor } from "@/proxy";
import { asOwner, describeDb, tx } from "./db";
import { shopOrganiser } from "./storefront-fixtures";

describeDb("preview links", () => {
  it("a new link opens the storefront without any organiser scope and expires after 7 days by default", async () => {
    const o = await shopOrganiser();
    const l = await tx(o.scope, (c) => createPreviewLink(c, o.sf.id, o.user.id));
    expect(l.token).toMatch(/^[A-Za-z0-9_-]{22}$/);
    const days = (l.expiresAt.getTime() - Date.now()) / 86_400_000;
    expect(days).toBeGreaterThan(6.99);
    expect(days).toBeLessThan(7.01);
    expect(await tx({}, (c) => resolvePreviewToken(c, l.token))).toEqual({ organiser_id: o.organiserId, storefront_id: o.sf.id });
  });

  it("revoked or expired links return not found", async () => {
    const o = await shopOrganiser();
    const a = await tx(o.scope, (c) => createPreviewLink(c, o.sf.id, o.user.id));
    await tx(o.scope, (c) => revokePreviewLink(c, o.sf.id, a.id));
    expect(await tx({}, (c) => resolvePreviewToken(c, a.token))).toBeNull();
    const b = await tx(o.scope, (c) => createPreviewLink(c, o.sf.id, o.user.id));
    await asOwner((c) => c.query("update control.preview_links set created_at = now() - interval '8 days', expires_at = now() - interval '1 day' where id = $1", [b.id]));
    expect(await tx({}, (c) => resolvePreviewToken(c, b.token))).toBeNull();
    const rows = await tx(o.scope, (c) => listPreviewLinks(c, o.sf.id));
    expect(rows.find((r) => r.id === a.id)!.revoked_at).not.toBeNull();
    expect(rows.find((r) => r.id === b.id)!.expired).toBe(true);
    // Another organiser cannot revoke it.
    const other = await shopOrganiser();
    const c2 = await tx(o.scope, (c) => createPreviewLink(c, o.sf.id, o.user.id));
    await expect(tx(other.scope, (c) => revokePreviewLink(c, o.sf.id, c2.id))).rejects.toMatchObject({ code: "not_found" });
    expect(await tx({}, (c) => resolvePreviewToken(c, c2.token))).not.toBeNull();
  });

  it("random tokens return not found, and the database holds only hashes of real tokens", async () => {
    const o = await shopOrganiser();
    const l = await tx(o.scope, (c) => createPreviewLink(c, o.sf.id, o.user.id));
    for (let i = 0; i < 50; i++) expect(await tx({}, (c) => resolvePreviewToken(c, randomBytes(16).toString("base64url")))).toBeNull();
    for (const junk of ["", "x", "../../etc/passwd", "' or 1=1 --", hashPreviewToken(l.token)]) expect(await tx({}, (c) => resolvePreviewToken(c, junk))).toBeNull();
    const stored = await asOwner(async (c) => (await c.query("select to_jsonb(l)::text as j from control.preview_links l where id = $1", [l.id])).rows[0].j as string);
    expect(stored).not.toContain(l.token);
    expect(stored).toContain(hashPreviewToken(l.token));
    // The web role cannot read preview links without an organiser scope.
    expect(await tx({}, async (c) => (await c.query("select count(*)::int as n from control.preview_links")).rows[0].n)).toBe(0);
  });

  it("preview links and the frame render without the platform chrome", () => {
    expect(layoutFor("/p/abc")).toBe("bare");
    expect(layoutFor("/studio/builder/frame/x")).toBe("bare");
    expect(layoutFor("/studio/builder")).toBe("wide");
    expect(layoutFor("/studio")).toBeNull();
  });
});
