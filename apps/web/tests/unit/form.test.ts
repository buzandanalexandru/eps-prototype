// Security review: Studio form parsing cannot pollute prototypes.
import { describe, expect, it } from "vitest";
import { errorState, formObject, indexed, UnsafeFieldError } from "@/lib/studio/form";

function fd(entries: [string, string][]) {
  const f = new FormData();
  for (const [k, v] of entries) f.append(k, v);
  return f;
}

describe("formObject", () => {
  it("builds nested objects and arrays", () => {
    const o = formObject(fd([["name.ro", "Galaxia"], ["name.en", "Galaxy"], ["days[]", "a"], ["days[]", "b"], ["steps.1.price", "2"], ["steps.0.price", "1"]]));
    expect(JSON.parse(JSON.stringify(o))).toEqual({ name: { ro: "Galaxia", en: "Galaxy" }, days: ["a", "b"], steps: { "0": { price: "1" }, "1": { price: "2" } } });
    expect(indexed<{ price: string }>(o.steps).map((s) => s.price)).toEqual(["1", "2"]);
  });

  for (const field of ["__proto__.x", "constructor.prototype.x", "a.__proto__.x", "prototype.x", "name.constructor", "a b", "x.999999.y"]) {
    it(`refuses ${field} and pollutes nothing`, () => {
      expect(() => formObject(fd([[field, "polluted"]]))).toThrow(UnsafeFieldError);
      expect(({} as Record<string, unknown>).x).toBeUndefined();
      expect((Object.prototype as unknown as Record<string, unknown>).x).toBeUndefined();
    });
  }

  it("objects have no prototype", () => {
    const o = formObject(fd([["a.b", "1"]]));
    expect(Object.getPrototypeOf(o)).toBeNull();
    expect(Object.getPrototypeOf(o.a)).toBeNull();
  });

  it("a scalar and a nested field with the same name are refused", () => {
    expect(() => formObject(fd([["a", "1"], ["a.b", "2"]]))).toThrow(UnsafeFieldError);
  });

  it("an unsafe field becomes a generic form error", () => {
    expect(errorState(new UnsafeFieldError("__proto__.x")).errors).toEqual({ _: "invalid" });
  });
});
