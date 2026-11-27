// Host classification (design D0.7). Platform hosts route by path (/studio, /s/<brand>, ...); every
// other host is a candidate custom brand domain, looked up in step 3.

export type HostKind = { kind: "platform"; host: string } | { kind: "brand"; host: string };

const PLATFORM_SUFFIXES = [".vercel.app"];
const PLATFORM_HOSTS = ["localhost", "127.0.0.1", "[::1]"];

export function classifyHost(rawHost: string | null, extra?: string): HostKind {
  const host = (rawHost || "").toLowerCase().trim().replace(/:\d+$/, "");
  const extras = (extra || "")
    .split(",")
    .map((h) => h.trim().toLowerCase())
    .filter(Boolean);
  if (
    host === "" ||
    PLATFORM_HOSTS.includes(host) ||
    extras.includes(host) ||
    PLATFORM_SUFFIXES.some((s) => host.endsWith(s))
  ) {
    return { kind: "platform", host };
  }
  return { kind: "brand", host };
}
