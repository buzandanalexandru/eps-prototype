// Domain errors carry a code that screens translate (errors.<code>) and tests assert on.
export class DomainError extends Error {
  constructor(
    readonly code: string,
    readonly field?: string,
    readonly params?: Record<string, string | number>,
  ) {
    super(code);
    this.name = "DomainError";
  }
}

export type FieldErrors = Record<string, string>;

/** Turns a pg error into a domain code: a broken link (FK) is "not found", a duplicate is "taken". */
export function fromPgError(err: unknown): DomainError | null {
  const e = err as { code?: string; constraint?: string } | null;
  if (!e || typeof e.code !== "string") return null;
  if (e.code === "23503") return new DomainError("not_found");
  if (e.code === "23505") return new DomainError("taken", e.constraint?.includes("slug") ? "slug" : undefined);
  if (e.code === "42501") return new DomainError("not_found"); // row-level security refused the row
  if (e.code === "23514") return new DomainError("invalid");
  if (e.code === "22P02") return new DomainError("not_found"); // malformed uuid in a URL
  return null;
}
