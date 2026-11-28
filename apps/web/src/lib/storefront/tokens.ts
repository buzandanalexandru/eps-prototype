// Design tokens to CSS custom properties (design D2.3). Every storefront component styles itself only
// with these variables (var(--eps-*)), so one renderer wears any brand. Values come only from the
// validated document: colours are #rrggbb, fonts and radii are ids mapped here, so nothing an
// organiser types reaches CSS as free text.
import { FONTS } from "./fonts";
import { PALETTE_KEYS, type Tokens } from "./schema";

export const RADIUS_PX = { none: "0px", s: "4px", m: "10px", l: "20px" } as const;

const kebab = (k: string) => k.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`);

/** The variables of one storefront root: both palettes (--eps-l-*, --eps-d-*), fonts and radius.
 *  The active palette (--eps-*) is chosen by the static rules in globals.css from data-eps-mode. */
export function tokenVars(tokens: Tokens): Record<string, string> {
  const out: Record<string, string> = {};
  for (const k of PALETTE_KEYS) {
    out[`--eps-l-${kebab(k)}`] = tokens.light[k];
    out[`--eps-d-${kebab(k)}`] = tokens.dark[k];
  }
  out["--eps-font-heading"] = FONTS[tokens.fontHeading].css;
  out["--eps-font-body"] = FONTS[tokens.fontBody].css;
  out["--eps-radius"] = RADIUS_PX[tokens.radius];
  return out;
}

/** The variables a section may use (the token test checks every section uses only these). */
export const TOKEN_VARIABLES = [
  ...PALETTE_KEYS.map((k) => `--eps-${kebab(k)}`),
  "--eps-font-heading",
  "--eps-font-body",
  "--eps-radius",
] as const;
