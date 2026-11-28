// The browser side of the shop's checkout, order and wallet calls (step 4): everything goes to our
// own origin (/api/shop/...), which forwards to the core with the visitor's core token.
import type { components } from "@/lib/core/schema";

export type OrderView = components["schemas"]["OrderView"];
export type Problem = components["schemas"]["Problem"];
export type Wallet = components["schemas"]["Wallet"];
export type Attempt = components["schemas"]["PaymentAttempt"];

export type Answer<T> = { ok: true; status: number; data: T } | { ok: false; status: number; problem: Problem };

export async function shopApi<T>(path: string, init: { method?: string; body?: unknown; key?: string; orderToken?: string; headers?: Record<string, string> } = {}): Promise<Answer<T>> {
  const headers: Record<string, string> = { ...(init.headers ?? {}) };
  if (init.body !== undefined) headers["Content-Type"] = "application/json";
  if (init.key) headers["Idempotency-Key"] = init.key;
  if (init.orderToken) headers["X-Order-Token"] = init.orderToken;
  try {
    const res = await fetch(`/api/shop/${path}`, {
      method: init.method ?? "GET",
      headers,
      body: init.body === undefined ? undefined : JSON.stringify(init.body),
      cache: "no-store",
      credentials: "same-origin",
    });
    // Our route answers 200 with the core's status and body inside (no browser network errors).
    const env = (await res.json().catch(() => null)) as { status?: number; body?: unknown } | null;
    const status = env?.status ?? 0;
    const json = env?.body;
    if (status >= 200 && status < 300) return { ok: true, status, data: json as T };
    const problem: Problem = json && typeof json === "object" && "code" in json ? (json as Problem) : { type: "", title: "", status, code: "EPS-UNAVAILABLE", requestId: "" };
    return { ok: false, status, problem };
  } catch {
    return { ok: false, status: 0, problem: { type: "", title: "", status: 0, code: "EPS-UNAVAILABLE", requestId: "", retryable: true } };
  }
}

/** A fresh idempotency key (one per button press, reused on retry). */
export function newKey(): string {
  return crypto.randomUUID();
}
