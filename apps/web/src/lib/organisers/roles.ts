// Team roles (design D1.6, A3 Thin): owner, admin, box office, finance, each with a scope. Only owner
// versus everyone else is enforced: fee policy, team changes and deleting a brand are owner-only.
import { DomainError } from "../errors";

export const ROLES = ["owner", "admin", "box_office", "finance"] as const;
export type Role = (typeof ROLES)[number];
export type ScopeType = "organiser" | "brand" | "event";

export function isRole(v: unknown): v is Role {
  return typeof v === "string" && (ROLES as readonly string[]).includes(v);
}

export const OWNER_ONLY = ["fee_policy", "team", "delete_brand"] as const;
export type OwnerAction = (typeof OWNER_ONLY)[number];

/** Throws owner_only unless the role may perform the owner action. */
export function assertOwner(role: string, action: OwnerAction): void {
  if (role !== "owner") throw new DomainError("owner_only", undefined, { action });
}

/** The role claim in core tokens (design D0.4): owner, admin, or staff for every other role. */
export function coreRoleClaim(role: string): "owner" | "admin" | "staff" {
  return role === "owner" ? "owner" : role === "admin" ? "admin" : "staff";
}

/** The last owner can be neither demoted nor removed. */
export function assertKeepsAnOwner(owners: number, targetIsOwner: boolean, nextRole: string | null): void {
  if (targetIsOwner && nextRole !== "owner" && owners <= 1) throw new DomainError("last_owner");
}
