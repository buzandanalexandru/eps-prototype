// Server-action plumbing for Studio forms: FormData → nested objects, and errors → a FormState the
// ActionForm component shows (codes translated on the client).
import * as z from "zod";
import { DomainError, fromPgError } from "../errors";
import { fieldErrors } from "../catalogue/schema";

export type FormState = {
  ok?: boolean;
  /** Message key under studio.saved / errors. */
  message?: string;
  errors?: Record<string, string>;
  params?: Record<string, string | number>;
  /** Extra data for the screen (e.g. an invitation link shown once). */
  data?: Record<string, string>;
  /** Bumps on every submission so the client can react to repeated identical results. */
  at?: number;
};

export const initialFormState: FormState = {};

/** Path segments that could reach an object's prototype; a field using one is refused. */
const FORBIDDEN_SEGMENTS = new Set(["__proto__", "constructor", "prototype"]);
const SEGMENT = /^[A-Za-z0-9_]{1,64}$/;

export class UnsafeFieldError extends Error {
  constructor(readonly field: string) {
    super("unsafe form field");
  }
}

/**
 * Builds an object from form fields: "name.ro" → { name: { ro } }, "days[]" → array of all values,
 * "steps.0.priceMinor" → steps[0].priceMinor. Unchecked checkboxes are absent. Every object is made
 * without a prototype, and field names may only use letters, digits and "_" per segment (never
 * __proto__, constructor or prototype), so a crafted field cannot pollute prototypes. Callers still
 * parse the result with a zod schema.
 */
export function formObject(fd: FormData): Record<string, unknown> {
  const out: Record<string, unknown> = Object.create(null);
  const keys = new Set<string>();
  for (const k of fd.keys()) if (!k.startsWith("$ACTION")) keys.add(k);
  for (const raw of keys) {
    const isArray = raw.endsWith("[]");
    const key = isArray ? raw.slice(0, -2) : raw;
    const path = key.split(".");
    if (path.length > 6 || path.some((p) => FORBIDDEN_SEGMENTS.has(p) || !SEGMENT.test(p))) throw new UnsafeFieldError(raw);
    const value = isArray ? fd.getAll(raw).map(String) : String(fd.get(raw));
    let node: Record<string, unknown> = out;
    path.forEach((p, i) => {
      if (i === path.length - 1) {
        node[p] = value;
        return;
      }
      if (/^\d+$/.test(p) && Number(p) > 400) throw new UnsafeFieldError(raw);
      const existing = node[p];
      if (existing === undefined) node[p] = Object.create(null);
      else if (typeof existing !== "object" || existing === null) throw new UnsafeFieldError(raw);
      node = node[p] as Record<string, unknown>;
    });
  }
  return out;
}

/** Turns an object whose keys are "0", "1", … (from formObject) into an array, in index order. */
export function indexed<T = unknown>(v: unknown): T[] {
  if (!v || typeof v !== "object") return [];
  return Object.keys(v)
    .filter((k) => /^\d+$/.test(k))
    .sort((a, b) => Number(a) - Number(b))
    .map((k) => (v as Record<string, T>)[k]);
}

export function errorState(err: unknown): FormState {
  if (err instanceof UnsafeFieldError) return { ok: false, errors: { _: "invalid" }, at: Date.now() };
  if (err instanceof z.ZodError) return { ok: false, errors: fieldErrors(err), at: Date.now() };
  const d = err instanceof DomainError ? err : fromPgError(err);
  if (d) return { ok: false, errors: { [d.field ?? "_"]: d.code }, params: d.params, at: Date.now() };
  console.error("studio action failed", (err as Error)?.message);
  return { ok: false, errors: { _: "unexpected" }, at: Date.now() };
}
