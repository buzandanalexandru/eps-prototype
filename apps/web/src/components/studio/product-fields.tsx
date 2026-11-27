import { getLocale, getTranslations } from "next-intl/server";
import type { DayRow, ProductRow } from "@/lib/catalogue/domain";
import type { ProductType } from "@/lib/catalogue/schema";
import { toLocalInput } from "@/lib/catalogue/time";
import { toMajorString } from "@/lib/pricing";
import { lt } from "@/lib/studio/text";
import { FieldError } from "./action-form";
import { Check, Hint, Input, Label, Select, Textarea } from "@/components/ui/field";

type Cfg = Record<string, unknown>;

async function DayPicker({ days, selected, name }: { days: DayRow[]; selected: string[]; name: string }) {
  const t = await getTranslations("products");
  const all = selected.length === 0 || selected.length === days.length;
  return (
    <fieldset className="space-y-1">
      <legend className="text-sm font-medium">{t("days")}</legend>
      <details open={days.length <= 14}>
        <summary className="cursor-pointer text-xs text-muted-foreground">{t("dayCount", { n: selected.length || days.length, of: days.length })}</summary>
        <div className="grid grid-cols-2 gap-1 pt-1 sm:grid-cols-4">
          {days.map((d) => (
            <Check key={d.day} name={name} value={d.day} defaultChecked={all || selected.includes(d.day)} label={d.day} />
          ))}
        </div>
      </details>
      <FieldError name="config.days" />
    </fieldset>
  );
}

export async function ProductFields({ type, product, days, others, tz }: { type: ProductType; product?: ProductRow; days: DayRow[]; others: ProductRow[]; tz: string }) {
  const t = await getTranslations("products");
  const locale = await getLocale();
  const cfg = (product?.config ?? {}) as Cfg;
  const perks = product?.perks ?? [];
  const requires = cfg.requires;
  return (
    <div className="space-y-3">
      <p className="rounded-md bg-muted p-2 text-sm" data-testid="type-explanation">
        <span className="font-medium">{t(`types.${type}.name`)}</span>: {t(`types.${type}.how`)}
      </p>
      <div className="grid gap-3 sm:grid-cols-2">
        <Label id="name-ro">
          {t("nameRo")}
          <Input name="name.ro" lang="ro" defaultValue={product?.name.ro} />
          <FieldError name="name.ro" />
        </Label>
        <Label id="name-en">
          {t("nameEn")}
          <Input name="name.en" lang="en" defaultValue={product?.name.en} />
          <FieldError name="name.en" />
        </Label>
        <Label>
          {t("perksRo")}
          <Textarea name="perks.ro" lang="ro" defaultValue={perks.map((p) => p.ro).join("\n")} />
        </Label>
        <Label>
          {t("perksEn")}
          <Textarea name="perks.en" lang="en" defaultValue={perks.map((p) => p.en).join("\n")} />
          <Hint>{t("perksHint")}</Hint>
        </Label>
      </div>

      {type === "pass" && (
        <>
          <Label>
            {t("zone")}
            <Input name="config.zone" defaultValue={String(cfg.zone ?? "")} />
            <FieldError name="config.zone" />
          </Label>
          <DayPicker days={days} selected={(cfg.days as string[]) ?? []} name="config.days" />
        </>
      )}
      {type === "flex_day" && <DayPicker days={days} selected={(cfg.days as string[]) ?? []} name="config.days" />}
      {type === "day_ticket" && (
        <Label>
          {t("day")}
          <Select name="config.day" defaultValue={String(cfg.day ?? "")}>
            <option value="">{t("pickAtPurchase")}</option>
            {days.map((d) => (
              <option key={d.day} value={d.day}>
                {d.day}
              </option>
            ))}
          </Select>
          <FieldError name="config.day" />
        </Label>
      )}
      {type === "add_on" && (
        <fieldset className="space-y-1">
          <legend className="text-sm font-medium">{t("requires")}</legend>
          {(["none", "any_ticket", "products"] as const).map((m) => (
            <label key={m} className="flex items-center gap-2 text-sm">
              <input type="radio" name="config.requiresMode" value={m} defaultChecked={(Array.isArray(requires) ? "products" : (requires ?? "any_ticket")) === m} />
              {t(`requiresMode.${m}`)}
            </label>
          ))}
          <div className="grid gap-1 pl-6 sm:grid-cols-2">
            {others
              .filter((o) => o.id !== product?.id && o.type !== "add_on")
              .map((o) => (
                <Check key={o.id} name="config.requires" value={o.id} defaultChecked={Array.isArray(requires) && requires.includes(o.id)} label={lt(o.name, locale) || o.type} />
              ))}
          </div>
          <FieldError name="config.requires" />
        </fieldset>
      )}
      {type === "voucher" && (
        <Label className="max-w-48">
          {t("spendValue")}
          <Input name="config.spendValue" inputMode="decimal" defaultValue={typeof cfg.spendValueMinor === "number" ? toMajorString(cfg.spendValueMinor) : ""} />
          <FieldError name="config.spendValueMinor" />
        </Label>
      )}
      {type === "table" && (
        <div className="grid gap-3 sm:grid-cols-2">
          <Label>
            {t("category")}
            <Input name="config.category" defaultValue={String(cfg.category ?? "")} />
            <FieldError name="config.category" />
          </Label>
          <Label>
            {t("seats")}
            <Input name="config.seats" inputMode="numeric" defaultValue={String(cfg.seats ?? "4")} />
            <FieldError name="config.seats" />
          </Label>
        </div>
      )}
      {type === "time_slot" && (
        <div className="grid gap-3 sm:grid-cols-4">
          <Label>
            {t("slotMinutes")}
            <Input name="config.slotMinutes" inputMode="numeric" defaultValue={String(cfg.slotMinutes ?? "30")} />
            <FieldError name="config.slotMinutes" />
          </Label>
          <Label>
            {t("firstStart")}
            <Input name="config.firstStart" type="time" defaultValue={String(cfg.firstStart ?? "17:00")} />
            <FieldError name="config.firstStart" />
          </Label>
          <Label>
            {t("lastStart")}
            <Input name="config.lastStart" type="time" defaultValue={String(cfg.lastStart ?? "22:00")} />
            <FieldError name="config.lastStart" />
          </Label>
          <Label>
            {t("maxConsecutive")}
            <Input name="config.maxConsecutiveSlots" inputMode="numeric" defaultValue={String(cfg.maxConsecutiveSlots ?? "4")} />
            <FieldError name="config.maxConsecutiveSlots" />
          </Label>
        </div>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        <Label>
          {t("salesFrom")}
          <Input name="salesFrom" type="datetime-local" defaultValue={product?.sales_from ? toLocalInput(product.sales_from, tz) : ""} />
          <FieldError name="salesFrom" />
        </Label>
        <Label>
          {t("salesUntil")}
          <Input name="salesUntil" type="datetime-local" defaultValue={product?.sales_until ? toLocalInput(product.sales_until, tz) : ""} />
          <FieldError name="salesUntil" />
        </Label>
        <Label>
          {t("maxPerOrder")}
          <Input name="maxPerOrder" inputMode="numeric" defaultValue={String(product?.max_per_order ?? 10)} />
          <FieldError name="maxPerOrder" />
        </Label>
        <Label>
          {t("sort")}
          <Input name="sort" inputMode="numeric" defaultValue={String(product?.sort ?? 0)} />
        </Label>
      </div>
      <div className="flex flex-wrap gap-4">
        <Check name="named" defaultChecked={product?.named ?? false} label={t("named")} />
        <Check name="visible" defaultChecked={product?.visible ?? true} label={t("visible")} />
      </div>
      <Hint>{t("namedHint")}</Hint>
    </div>
  );
}
