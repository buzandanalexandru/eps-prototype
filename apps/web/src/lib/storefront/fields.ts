// Field constructors for section and block settings. Each one is a zod schema carrying a small
// descriptor in `fieldRegistry`, so the editor can generate a form from a section's schema
// (design D2.2) and the publish check knows which texts are required.
import * as z from "zod";
import { isSafeLink, parseImageRef } from "./links";

export type FieldKind = "localized" | "markdown" | "enum" | "bool" | "int" | "link" | "image" | "event" | "datetime";
export type FieldMeta = {
  kind: FieldKind;
  /** Needed before publishing (texts: in every enabled language). */
  required?: boolean;
  max?: number;
  min?: number;
  options?: readonly string[];
};

export const fieldRegistry = z.registry<FieldMeta>();

// No control characters except newlines and tabs in text (they have no business in a shop).
const plain = (max: number) =>
  z
    .string()
    .max(max, "too_long")
    .refine((s) => !/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/.test(s), "invalid");

export const localized = (max: number, required = false) =>
  z.strictObject({ ro: plain(max), en: plain(max) }).register(fieldRegistry, { kind: "localized", max, required });

export const markdown = (max: number, required = false) =>
  z.strictObject({ ro: plain(max), en: plain(max) }).register(fieldRegistry, { kind: "markdown", max, required });

export const choice = <T extends readonly [string, ...string[]]>(options: T) =>
  z.enum(options).register(fieldRegistry, { kind: "enum", options });

export const flag = () => z.boolean().register(fieldRegistry, { kind: "bool" });

export const int = (min: number, max: number) => z.int().min(min).max(max).register(fieldRegistry, { kind: "int", min, max });

/** "" (no link) or a safe link. */
export const link = (required = false) =>
  z
    .string()
    .max(500)
    .refine((s) => s === "" || isSafeLink(s), "unsafe_link")
    .register(fieldRegistry, { kind: "link", required });

/** "" (no image), "asset:<uuid>" or "sample:<name>". */
export const image = (required = false) =>
  z
    .string()
    .max(60)
    .refine((s) => s === "" || parseImageRef(s) !== null, "unsafe_image")
    .register(fieldRegistry, { kind: "image", required });

/** "" or the id of one of the organiser's events (checked against the catalogue on save). */
export const eventRef = (required = false) =>
  z
    .string()
    .regex(/^$|^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/, "not_found")
    .register(fieldRegistry, { kind: "event", required });

/** "" or an ISO date-time with an offset. */
export const datetime = () =>
  z
    .string()
    .regex(/^$|^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2})?(\.\d+)?(Z|[+-]\d{2}:\d{2})$/, "date_invalid")
    .register(fieldRegistry, { kind: "datetime" });

export type FieldDescriptor = FieldMeta & { key: string };

/** The editable fields of a settings object, in declaration order. */
export function describeFields(schema: z.ZodObject): FieldDescriptor[] {
  return Object.entries(schema.shape).flatMap(([key, s]) => {
    const meta = fieldRegistry.get(s as z.ZodType);
    return meta ? [{ key, ...meta }] : [];
  });
}
