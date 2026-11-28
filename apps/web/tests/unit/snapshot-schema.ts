// A small validator for the JSON Schema keywords api/schemas/sale-snapshot.v1.json uses, so the web
// side tests compiled documents against the shared contract without a dependency (the Go side has
// the same in services/internal/snapshot/schemacheck_test.go).
import { readFileSync } from "node:fs";
import path from "node:path";

type Schema = Record<string, unknown>;

export const SNAPSHOT_SCHEMA: Schema = JSON.parse(readFileSync(path.resolve(__dirname, "../../../../api/schemas/sale-snapshot.v1.json"), "utf8"));

function typeOf(v: unknown): string {
  if (v === null) return "null";
  if (Array.isArray(v)) return "array";
  if (typeof v === "number") return Number.isInteger(v) ? "integer" : "number";
  return typeof v;
}

function typeOk(want: unknown, got: string): boolean {
  const ok = (w: unknown) => w === got || (w === "number" && got === "integer");
  return Array.isArray(want) ? want.some(ok) : ok(want);
}

export function validateSnapshot(v: unknown, root: Schema = SNAPSHOT_SCHEMA): string | null {
  return check(root, root, v, "$");
}

function check(root: Schema, s: Schema, v: unknown, p: string): string | null {
  if (typeof s.$ref === "string") {
    const def = (root.$defs as Record<string, Schema>)[s.$ref.replace("#/$defs/", "")];
    return def ? check(root, def, v, p) : `${p}: unknown $ref ${s.$ref}`;
  }
  if (Array.isArray(s.oneOf)) {
    const errs = (s.oneOf as Schema[]).map((o) => check(root, o, v, p));
    const n = errs.filter((e) => e === null).length;
    return n === 1 ? null : `${p}: matches ${n} of oneOf (${errs.find((e) => e) ?? ""})`;
  }
  const got = typeOf(v);
  if (s.type !== undefined && !typeOk(s.type, got)) return `${p}: type ${got}, want ${JSON.stringify(s.type)}`;
  if (Array.isArray(s.enum) && !s.enum.includes(v as never)) return `${p}: ${JSON.stringify(v)} not in enum`;
  if (typeof v === "number") {
    if (typeof s.minimum === "number" && v < s.minimum) return `${p}: below ${s.minimum}`;
    if (typeof s.maximum === "number" && v > s.maximum) return `${p}: above ${s.maximum}`;
  }
  if (typeof v === "string") {
    if (typeof s.minLength === "number" && v.length < s.minLength) return `${p}: too short`;
    if (typeof s.pattern === "string" && !new RegExp(s.pattern).test(v)) return `${p}: ${v} does not match ${s.pattern}`;
  }
  if (Array.isArray(v)) {
    if (typeof s.minItems === "number" && v.length < s.minItems) return `${p}: fewer than ${s.minItems} items`;
    if (s.items) for (const [i, e] of v.entries()) {
      const err = check(root, s.items as Schema, e, `${p}[${i}]`);
      if (err) return err;
    }
  }
  if (got === "object") {
    const o = v as Record<string, unknown>;
    const props = (s.properties ?? {}) as Record<string, Schema>;
    for (const r of (s.required as string[] | undefined) ?? []) if (!(r in o)) return `${p}: missing ${r}`;
    for (const [k, e] of Object.entries(o)) {
      if (!Object.prototype.hasOwnProperty.call(props, k)) {
        if (s.additionalProperties === false) return `${p}: unexpected ${k}`;
        continue;
      }
      const err = check(root, props[k], e, `${p}.${k}`);
      if (err) return err;
    }
  }
  return null;
}
