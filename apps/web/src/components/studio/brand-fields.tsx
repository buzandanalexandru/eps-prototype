import { getTranslations } from "next-intl/server";
import type { BrandRow } from "@/lib/organisers/domain";
import { TEMPLATES } from "@/lib/organisers/domain";
import { FieldError } from "./action-form";
import { Check, Hint, Input, Label, Select } from "@/components/ui/field";

export async function BrandFields({ brand, withTemplate }: { brand?: BrandRow; withTemplate?: boolean }) {
  const t = await getTranslations("brands");
  const langs = brand?.languages ?? ["ro", "en"];
  return (
    <>
      <Label>
        {t("name")}
        <Input name="name" required maxLength={120} defaultValue={brand?.name} />
        <FieldError name="name" />
      </Label>
      <Label>
        {t("slug")}
        <Input name="slug" required autoCapitalize="none" spellCheck={false} defaultValue={brand?.slug} />
        <Hint>{t("slugHint")}</Hint>
        <FieldError name="slug" />
      </Label>
      <fieldset className="space-y-1">
        <legend className="text-sm font-medium">{t("languages")}</legend>
        <div className="flex flex-wrap gap-4">
          <Check name="languages" value="ro" defaultChecked={langs.includes("ro")} label={t("lang.ro")} />
          <Check name="languages" value="en" defaultChecked={langs.includes("en")} label={t("lang.en")} />
        </div>
        <FieldError name="languages" />
      </fieldset>
      <Label>
        {t("currency")}
        <Select name="currency" defaultValue={brand?.currency ?? "RON"}>
          <option value="RON">RON</option>
          <option value="EUR">EUR</option>
        </Select>
      </Label>
      {withTemplate ? (
        <fieldset className="space-y-2">
          <legend className="text-sm font-medium">{t("template")}</legend>
          <Hint>{t("templateHint")}</Hint>
          <div className="grid gap-2 sm:grid-cols-2">
            {TEMPLATES.map((tpl, i) => (
              <label key={tpl} className="flex items-start gap-2 rounded-md border p-2 text-sm">
                <input type="radio" name="template" value={tpl} defaultChecked={(brand?.template ?? "festival") === tpl || (!brand && i === 0)} className="mt-1" />
                <span>
                  <span className="font-medium">{t(`templates.${tpl}.name`)}</span>
                  <span className="block text-xs text-muted-foreground">{t(`templates.${tpl}.lead`)}</span>
                </span>
              </label>
            ))}
          </div>
        </fieldset>
      ) : (
        <input type="hidden" name="template" value={brand?.template ?? "festival"} />
      )}
      <Label>
        {t("logo")}
        <Input name="logoUrl" type="url" placeholder="https://" defaultValue={brand?.logo_url ?? ""} />
        <Hint>{t("logoHint")}</Hint>
        <FieldError name="logoUrl" />
      </Label>
    </>
  );
}
