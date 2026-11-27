import { describe, expect, it } from "vitest";
import { createElement } from "react";
import { useTranslations } from "next-intl";
import { renderWithIntl } from "./render";
import en from "../../messages/en.json";
import ro from "../../messages/ro.json";
import { resolveLocale } from "@/i18n/locale";

const messages = { en, ro };

function Title() {
  const t = useTranslations("signIn");
  return createElement("h1", null, t("title"), " / ", t("demoFan"));
}

function render(cookie: string | undefined, acceptLanguage: string | null) {
  const locale = resolveLocale(cookie, acceptLanguage);
  return renderWithIntl(locale, messages[locale], createElement(Title));
}

describe("languages", () => {
  it("the cookie picks the language: Romanian with diacritics, then English", () => {
    expect(render("ro", "en-GB,en;q=0.9")).toContain("Autentifică-te în EPS");
    expect(render("ro", null)).toContain("Continuă ca fan demo");
    expect(render("en", "ro-RO")).toContain("Sign in to EPS");
  });

  it("without a cookie, Accept-Language decides, then Romanian", () => {
    expect(resolveLocale(undefined, "en-US,en;q=0.9,ro;q=0.8")).toBe("en");
    expect(resolveLocale(undefined, "de-DE,ro;q=0.7,en;q=0.5")).toBe("ro");
    expect(resolveLocale(undefined, "fr;q=1, en;q=0")).toBe("ro");
    expect(resolveLocale("de", null)).toBe("ro");
  });

  it("both languages have the same keys and Romanian uses comma-below ș and ț", () => {
    const keys = (o: object, p = ""): string[] =>
      Object.entries(o).flatMap(([k, v]) => (typeof v === "object" ? keys(v, `${p}${k}.`) : [`${p}${k}`]));
    expect(keys(ro).sort()).toEqual(keys(en).sort());
    const text = JSON.stringify(ro);
    expect(text).not.toMatch(/[şţŞŢ]/); // cedilla forms are wrong in Romanian
    expect(text).toMatch(/[ăâîșț]/);
  });
});
