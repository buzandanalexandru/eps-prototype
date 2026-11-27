// Organisers spec "Active organiser": switching changes what Studio sees and the next core token.
import { describe, expect, it } from "vitest";
import { acceptInvitation, inviteMember, listBrands, pickActive, setActiveOrganiser, userOrganisers } from "@/lib/organisers/domain";
import { cachedMemberships, dbMembershipLookup } from "@/lib/auth/membership";
import { issueCoreToken, type AuthLike } from "@/lib/auth/core-token";
import { asOwner, describeDb, makeOrganiser, testPool, tx } from "./db";

describe("picking the active organiser", () => {
  it("uses the session's choice while the user is a member, else the oldest", () => {
    const orgs = [{ id: "a" }, { id: "b" }];
    expect(pickActive(orgs, "b")).toEqual({ id: "b" });
    expect(pickActive(orgs, "gone")).toEqual({ id: "a" });
    expect(pickActive([], "b")).toBeNull();
  });
});

describeDb("switching organiser", () => {
  it("a user in A and B switches to B: Studio shows only B's brands and the next core token carries B", async () => {
    const a = await makeOrganiser();
    const b = await makeOrganiser();
    const user = a.user;
    const inv = await tx(b.scope, (c) => inviteMember(c, { role: "owner", userId: b.user.id, input: { email: user.email, role: "admin" } }));
    await tx({ organiserId: b.organiserId, userId: user.id }, (c) => acceptInvitation(c, { organiserId: b.organiserId, token: inv.token, userId: user.id, email: user.email }));
    const orgs = await tx({ userId: user.id }, (c) => userOrganisers(c, user.id));
    expect(orgs.map((o) => o.id)).toEqual([a.organiserId, b.organiserId]);

    const sessionId = `s_${Date.now()}`;
    await asOwner((c) =>
      c.query(`insert into control."session" (id, "expiresAt", token, "updatedAt", "userId") values ($1, now() + interval '1 day', $1, now(), $2)`, [sessionId, user.id]),
    );
    await tx({ userId: user.id }, (c) => setActiveOrganiser(c, { sessionId, userId: user.id, organiserId: b.organiserId }));
    const active = (await asOwner((c) => c.query(`select "activeOrganiserId" from control."session" where id = $1`, [sessionId]))).rows[0].activeOrganiserId;
    expect(active).toBe(b.organiserId);
    const brands = await tx({ organiserId: active, userId: user.id }, (c) => listBrands(c));
    expect(brands.map((x) => x.id)).toEqual([b.brandId]);

    // The core token follows the session's active organiser at once (the cache is keyed by it).
    const lookup = cachedMemberships(dbMembershipLookup(testPool()));
    const auth: AuthLike = {
      api: {
        getSession: async () => ({ session: { id: sessionId, activeOrganiserId: active }, user: { id: user.id } }),
        signJWT: async ({ body }) => ({ token: Buffer.from(JSON.stringify(body.payload)).toString("base64url") }),
      },
    };
    expect(await lookup(user.id, a.organiserId)).toEqual({ organiserId: a.organiserId, role: "owner" });
    const res = await issueCoreToken({ auth, memberships: lookup, secret: "x".repeat(32), secureCookies: true }, { headers: new Headers(), guest: false });
    if (res.status !== 200) throw new Error("no token");
    expect(JSON.parse(Buffer.from(res.token, "base64url").toString())).toMatchObject({ kind: "organiser", org: b.organiserId, role: "admin" });
  });

  it("cannot switch to an organiser the user does not belong to", async () => {
    const a = await makeOrganiser();
    const b = await makeOrganiser();
    await expect(tx({ userId: a.user.id }, (c) => setActiveOrganiser(c, { sessionId: "x", userId: a.user.id, organiserId: b.organiserId }))).rejects.toMatchObject({ code: "not_found" });
    // A stale choice falls back to a real membership in the token.
    expect(await dbMembershipLookup(testPool())(a.user.id, b.organiserId)).toEqual({ organiserId: a.organiserId, role: "owner" });
  });
});
