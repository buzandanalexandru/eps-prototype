// Organisers spec "Team roles screen": owner-only actions and the last-owner rule.
import { describe, expect, it } from "vitest";
import { acceptInvitation, changeRole, deleteBrand, inviteMember, listInvitations, listMembers, removeMember, setFeePolicy } from "@/lib/organisers/domain";
import { assertOwner, coreRoleClaim } from "@/lib/organisers/roles";
import { describeDb, makeOrganiser, makeUser, tx } from "./db";
import { festival } from "./catalog-fixtures";

describe("role rules", () => {
  it("only the owner passes owner checks", () => {
    expect(() => assertOwner("owner", "team")).not.toThrow();
    for (const r of ["admin", "box_office", "finance", "staff"]) expect(() => assertOwner(r, "fee_policy")).toThrow("owner_only");
  });
  it("maps roles to the core token's claim", () => {
    expect([coreRoleClaim("owner"), coreRoleClaim("admin"), coreRoleClaim("box_office"), coreRoleClaim("finance")]).toEqual(["owner", "admin", "staff", "staff"]);
  });
});

describeDb("team in the database", () => {
  async function withAdmin() {
    const o = await makeOrganiser();
    const admin = await makeUser();
    const inv = await tx(o.scope, (c) => inviteMember(c, { role: "owner", userId: o.user.id, input: { email: admin.email, role: "admin" } }));
    await tx({ organiserId: o.organiserId, userId: admin.id }, (c) => acceptInvitation(c, { organiserId: o.organiserId, token: inv.token, userId: admin.id, email: admin.email }));
    return { o, admin };
  }

  it("an admin trying to change the fee policy or the team is refused with owner_only", async () => {
    const { o, admin } = await withAdmin();
    const scope = { organiserId: o.organiserId, userId: admin.id };
    await expect(tx(scope, (c) => setFeePolicy(c, "admin", "absorb"))).rejects.toMatchObject({ code: "owner_only" });
    await expect(tx(scope, (c) => inviteMember(c, { role: "admin", userId: admin.id, input: { email: "x@example.com", role: "finance" } }))).rejects.toMatchObject({ code: "owner_only" });
    await expect(tx(scope, (c) => changeRole(c, { role: "admin", targetUserId: o.user.id, input: { role: "finance" } }))).rejects.toMatchObject({ code: "owner_only" });
    await expect(tx(scope, (c) => removeMember(c, { role: "admin", targetUserId: o.user.id }))).rejects.toMatchObject({ code: "owner_only" });
    await expect(tx(scope, (c) => deleteBrand(c, "admin", o.brandId))).rejects.toMatchObject({ code: "owner_only" });
  });

  it("the only owner cannot demote or remove themselves; with a second owner they can", async () => {
    const { o, admin } = await withAdmin();
    await expect(tx(o.scope, (c) => changeRole(c, { role: "owner", targetUserId: o.user.id, input: { role: "admin" } }))).rejects.toMatchObject({ code: "last_owner" });
    await expect(tx(o.scope, (c) => removeMember(c, { role: "owner", targetUserId: o.user.id }))).rejects.toMatchObject({ code: "last_owner" });
    await tx(o.scope, (c) => changeRole(c, { role: "owner", targetUserId: admin.id, input: { role: "owner" } }));
    await tx(o.scope, (c) => changeRole(c, { role: "owner", targetUserId: o.user.id, input: { role: "finance" } }));
    const members = await tx(o.scope, (c) => listMembers(c));
    expect(members.map((m) => [m.user_id, m.role]).sort()).toEqual([[admin.id, "owner"], [o.user.id, "finance"]].sort());
  });

  it("invites box office for one event: a pending link that only the invited address can accept, once", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    const door = await makeUser(`door-${Date.now()}@example.com`);
    const other = await makeUser();
    const inv = await tx(o.scope, (c) => inviteMember(c, { role: "owner", userId: o.user.id, input: { email: door.email.toUpperCase(), role: "box_office", scopeType: "event", scopeId: f.ev.id } }));
    expect(await tx(o.scope, (c) => listInvitations(c))).toEqual([expect.objectContaining({ email: door.email, role: "box_office", scope_type: "event", scope_id: f.ev.id })]);
    await expect(tx({ organiserId: o.organiserId, userId: other.id }, (c) => acceptInvitation(c, { organiserId: o.organiserId, token: inv.token, userId: other.id, email: other.email }))).rejects.toMatchObject({ code: "invitation_other_email" });
    await expect(tx({ organiserId: o.organiserId, userId: door.id }, (c) => acceptInvitation(c, { organiserId: o.organiserId, token: "wrong", userId: door.id, email: door.email }))).rejects.toMatchObject({ code: "invitation_invalid" });
    await tx({ organiserId: o.organiserId, userId: door.id }, (c) => acceptInvitation(c, { organiserId: o.organiserId, token: inv.token, userId: door.id, email: door.email }));
    const members = await tx(o.scope, (c) => listMembers(c));
    expect(members.find((m) => m.user_id === door.id)).toMatchObject({ role: "box_office", scope_type: "event", scope_id: f.ev.id });
    await expect(tx({ organiserId: o.organiserId, userId: door.id }, (c) => acceptInvitation(c, { organiserId: o.organiserId, token: inv.token, userId: door.id, email: door.email }))).rejects.toMatchObject({ code: "invitation_invalid" });
    expect(await tx(o.scope, (c) => listInvitations(c))).toEqual([]);
  });

  it("an invitation token for organiser A does nothing when presented with organiser B's id", async () => {
    const a = await makeOrganiser();
    const b = await makeOrganiser();
    const u = await makeUser();
    const inv = await tx(a.scope, (c) => inviteMember(c, { role: "owner", userId: a.user.id, input: { email: u.email, role: "admin" } }));
    await expect(tx({ organiserId: b.organiserId, userId: u.id }, (c) => acceptInvitation(c, { organiserId: b.organiserId, token: inv.token, userId: u.id, email: u.email }))).rejects.toMatchObject({ code: "invitation_invalid" });
  });

  it("team listing never shows another organiser's members", async () => {
    const a = await makeOrganiser();
    const b = await makeOrganiser();
    const members = await tx(a.scope, (c) => listMembers(c));
    expect(members.map((m) => m.user_id)).toEqual([a.user.id]);
    expect(members.map((m) => m.user_id)).not.toContain(b.user.id);
  });
});
