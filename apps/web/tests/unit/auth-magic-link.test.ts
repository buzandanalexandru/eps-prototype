import { afterEach, describe, expect, it, vi } from "vitest";
import { cookieFrom, DOMAIN, get, makeAuth, ORIGIN, post } from "./helpers";

afterEach(() => {
  vi.useRealTimers();
});

async function requestLink(auth: ReturnType<typeof makeAuth>, email: string) {
  const res = await auth.auth.handler(
    post("/sign-in/magic-link", { email, callbackURL: "/studio", errorCallbackURL: "/sign-in?next=%2Fstudio" }),
  );
  return res;
}

describe("magic-link sign-in", () => {
  it("first sign-in creates the user, starts a session and lands on the page they came from", async () => {
    const a = makeAuth();
    const email = `new-person@${DOMAIN}`;
    expect((await requestLink(a, email)).status).toBe(200);
    const [entry] = await a.store.latest(email);
    expect(entry).toBeDefined();
    expect(a.db.user).toHaveLength(0);

    const res = await a.auth.handler(get(entry.url));
    expect(res.status).toBe(302);
    expect(res.headers.get("location")).toBe(`${ORIGIN}/studio`);
    const cookie = cookieFrom(res);
    expect(cookie).toContain("session_token");
    expect(a.db.user).toHaveLength(1);
    const session = await a.auth.api.getSession({ headers: new Headers({ cookie }) });
    expect(session?.user.email).toBe(email);
    expect(session?.user.emailVerified).toBe(true);
  });

  it("a reused link starts no session and offers a new link", async () => {
    const a = makeAuth();
    const email = `twice@${DOMAIN}`;
    await requestLink(a, email);
    const [entry] = await a.store.latest(email);
    expect((await a.auth.handler(get(entry.url))).status).toBe(302);

    const again = await a.auth.handler(get(entry.url));
    expect(again.status).toBe(302);
    expect(again.headers.get("location")).toMatch(/^http:\/\/localhost:3000\/sign-in\?next=%2Fstudio&error=/);
    expect(cookieFrom(again)).not.toContain("session_token");
  });

  it("an expired link (older than 15 minutes) starts no session", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-11-27T12:00:00Z"));
    const a = makeAuth();
    const email = `late@${DOMAIN}`;
    await requestLink(a, email);
    const [entry] = await a.store.latest(email);

    vi.setSystemTime(new Date("2026-11-27T12:15:30Z"));
    const res = await a.auth.handler(get(entry.url));
    expect(res.headers.get("location")).toMatch(/error=/);
    expect(cookieFrom(res)).not.toContain("session_token");
    expect(a.db.session).toHaveLength(0);
  });

  it("a link opened within 15 minutes still works", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-11-27T12:00:00Z"));
    const a = makeAuth();
    const email = `in-time@${DOMAIN}`;
    await requestLink(a, email);
    const [entry] = await a.store.latest(email);
    vi.setSystemTime(new Date("2026-11-27T12:14:30Z"));
    const res = await a.auth.handler(get(entry.url));
    expect(res.headers.get("location")).toBe(`${ORIGIN}/studio`);
  });

  it("without a mail provider, real addresses get no link and nothing is stored", async () => {
    const a = makeAuth();
    const res = await requestLink(a, "someone@example.com");
    expect(res.status).toBeGreaterThanOrEqual(400);
    expect(a.store.entries).toHaveLength(0);
  });

  it("stores only a hash of the token", async () => {
    const a = makeAuth();
    const email = `hashed@${DOMAIN}`;
    await requestLink(a, email);
    const [entry] = await a.store.latest(email);
    const token = new URL(entry.url).searchParams.get("token")!;
    const stored = JSON.stringify(a.db.verification);
    expect(stored).not.toContain(token);
  });
});
