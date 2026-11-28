// The browser's calls to our own origin's /api/fan and /api/door forwarders (always HTTP 200 with
// {status, body} inside).
export type Answer<T> = { status: number; body: T & { code?: string; data?: Record<string, unknown> } };

export async function call<T>(method: "GET" | "POST" | "PUT", url: string, body?: unknown): Promise<Answer<T>> {
  try {
    const res = await fetch(url, {
      method,
      headers: body === undefined ? { Accept: "application/json" } : { "Content-Type": "application/json", Accept: "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
      cache: "no-store",
    });
    if (!res.ok) return { status: res.status, body: { code: "EPS-UNAVAILABLE" } as T & { code: string } };
    return (await res.json()) as Answer<T>;
  } catch {
    return { status: 0, body: { code: "EPS-OFFLINE" } as T & { code: string } };
  }
}
