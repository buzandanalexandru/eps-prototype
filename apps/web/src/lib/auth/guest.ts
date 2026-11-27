// Guest identities (design D0.4): a random id in a signed, http-only cookie. Claiming a guest into
// an account is step 4.
import { createHmac, randomUUID, timingSafeEqual } from "node:crypto";

export const GUEST_COOKIE = "eps_guest";
export const GUEST_COOKIE_MAX_AGE = 30 * 24 * 3600;

function mac(id: string, secret: string) {
  return createHmac("sha256", `${secret}:eps-guest`).update(id).digest("base64url");
}

export function newGuestId() {
  return randomUUID();
}

export function signGuest(id: string, secret: string) {
  return `${id}.${mac(id, secret)}`;
}

/** Returns the guest id when the cookie value is authentic, else null. */
export function verifyGuest(value: string | undefined, secret: string): string | null {
  if (!value) return null;
  const dot = value.lastIndexOf(".");
  if (dot <= 0) return null;
  const id = value.slice(0, dot);
  if (!/^[0-9a-f-]{36}$/.test(id)) return null;
  const want = Buffer.from(mac(id, secret));
  const got = Buffer.from(value.slice(dot + 1));
  return want.length === got.length && timingSafeEqual(want, got) ? id : null;
}

export function guestCookie(value: string, secure: boolean) {
  return [
    `${GUEST_COOKIE}=${value}`,
    "Path=/",
    "HttpOnly",
    "SameSite=Lax",
    `Max-Age=${GUEST_COOKIE_MAX_AGE}`,
    ...(secure ? ["Secure"] : []),
  ].join("; ");
}
