// Organisers spec "Organiser and brand slugs are unique and safe".
import { describe, expect, it } from "vitest";
import { checkSlug, slugProblem, suggestSlug } from "@/lib/organisers/slugs";
import { createBrand } from "@/lib/organisers/domain";
import { DomainError } from "@/lib/errors";
import { describeDb, makeOrganiser, tx, uniq } from "./db";

describe("slug rules", () => {
  it("refuses reserved, short, uppercase, spaced and diacritic slugs with the rule they break", () => {
    expect(slugProblem("studio")).toBe("slug_reserved");
    expect(slugProblem("fees")).toBe("slug_reserved");
    expect(slugProblem("Ab")).toBe("slug_length");
    expect(slugProblem("Abc")).toBe("slug_characters");
    expect(slugProblem("my shop")).toBe("slug_characters");
    expect(slugProblem("brașov")).toBe("slug_characters");
    expect(slugProblem("-abc")).toBe("slug_hyphen");
    expect(slugProblem("a".repeat(41))).toBe("slug_length");
    expect(slugProblem("sunwave-events")).toBeNull();
  });
  it("suggests a slug without diacritics", () => {
    expect(suggestSlug("Berăria NIBIRU Țară")).toBe("beraria-nibiru-tara");
  });
});

describeDb("slug availability across organisers", () => {
  it("a brand slug held by another organiser is taken, and the answer says nothing else", async () => {
    const a = await makeOrganiser();
    const b = await makeOrganiser();
    const slug = uniq("held");
    await tx(a.scope, (c) => createBrand(c, a.organiserId, { name: "Held", slug, languages: ["ro"], currency: "RON" }));
    // Organiser B cannot see A's brand…
    const seen = await tx(b.scope, async (c) => (await c.query("select count(*)::int as n from control.brands where slug = $1", [slug])).rows[0].n);
    expect(seen).toBe(0);
    // …but the availability check says the address is taken, as a bare boolean.
    const raw = await tx(b.scope, async (c) => (await c.query("select control.slug_available('brand', $1) as r", [slug])).rows[0]);
    expect(raw).toEqual({ r: false });
    expect(await tx(b.scope, (c) => checkSlug(c, "brand", slug))).toBe("slug_taken");
    await expect(tx(b.scope, (c) => createBrand(c, b.organiserId, { name: "Mine", slug, languages: ["ro"], currency: "RON" }))).rejects.toMatchObject({
      code: "slug_taken",
      field: "slug",
    } satisfies Partial<DomainError>);
  });
  it("free, reserved and malformed slugs", async () => {
    const a = await makeOrganiser();
    expect(await tx(a.scope, (c) => checkSlug(c, "brand", uniq("free")))).toBe("ok");
    expect(await tx(a.scope, (c) => checkSlug(c, "brand", "studio"))).toBe("slug_reserved");
    expect(await tx(a.scope, (c) => checkSlug(c, "organiser", "Ab"))).toBe("slug_length");
  });
  it("the function runs as its own role that cannot bypass row-level security", async () => {
    const a = await makeOrganiser();
    const r = await tx(a.scope, async (c) =>
      (
        await c.query(`select r.rolname, r.rolbypassrls, r.rolcanlogin, p.prosecdef, p.proconfig
          from pg_proc p join pg_roles r on r.oid = p.proowner where p.oid = 'control.slug_available(text,text)'::regprocedure`)
      ).rows[0],
    );
    expect(r).toMatchObject({ rolname: "eps_slug_check", rolbypassrls: false, rolcanlogin: false, prosecdef: true });
    expect(r.proconfig.join(",")).toContain("search_path=pg_catalog, pg_temp");
  });
});
