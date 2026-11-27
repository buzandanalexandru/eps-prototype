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

/**
 * Builds an object from form fields: "name.ro" → { name: { ro } }, "days[]" → array of all values,
 * "steps.0.priceMinor" → steps[0].priceMinor. Unchecked checkboxes are absent.
 */
export function formObject(fd: FormData): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  const keys = new Set<string>();
  for (const k of fd.keys()) if (!k.startsWith("$ACTION")) keys.add(k);
  for (const raw of keys) {
    const isArray = raw.endsWith("[]");
    const key = isArray ? raw.slice(0, -2) : raw;
    const value = isArray ? fd.getAll(raw).map(String) : String(fd.get(raw));
    const path = key.split(".");
    let node: Record<string, unknown> | unknown[] = out;
    path.forEach((p, i) => {
      const last = i === path.length - 1;
      const nextIsIndex = !last && /^\d+$/.test(path[i + 1]);
      const slot = node as Record<string, unknown>;
      if (last) slot[p] = value;
      else {
        slot[p] ??= nextIsIndex ? [] : {};
        node = slot[p] as Record<string, unknown>;
      }
    });
  }
  return out;
}

export function errorState(err: unknown): FormState {
  if (err instanceof z.ZodError) return { ok: false, errors: fieldErrors(err), at: Date.now() };
  const d = err instanceof DomainError ? err : fromPgError(err);
  if (d) return { ok: false, errors: { [d.field ?? "_"]: d.code }, params: d.params, at: Date.now() };
  console.error("studio action failed", (err as Error)?.message);
  return { ok: false, errors: { _: "unexpected" }, at: Date.now() };
}
