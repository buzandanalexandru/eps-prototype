import { getTranslations } from "next-intl/server";
import type { EventRow, VenueRow } from "@/lib/catalogue/domain";
import type { BrandRow } from "@/lib/organisers/domain";
import { DEFAULT_TIMEZONE, toLocalInput } from "@/lib/catalogue/time";
import { FieldError } from "./action-form";
import { Hint, Input, Label, Select, Textarea } from "@/components/ui/field";

export async function EventFields({ ev, brands, venues, isNew }: { ev?: EventRow; brands: BrandRow[]; venues: VenueRow[]; isNew?: boolean }) {
  const t = await getTranslations("events");
  const tz = ev?.timezone ?? DEFAULT_TIMEZONE;
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <Label>
        {t("brand")}
        <Select name="brandId" defaultValue={ev?.brand_id ?? brands[0]?.id}>
          {brands.map((b) => (
            <option key={b.id} value={b.id}>
              {b.name}
            </option>
          ))}
        </Select>
        <FieldError name="brandId" />
      </Label>
      <Label id="venueId">
        {t("venue")}
        <Select name="venueId" defaultValue={ev?.venue_id ?? ""}>
          <option value="">{t("noVenue")}</option>
          {venues.map((v) => (
            <option key={v.id} value={v.id}>
              {v.name}
            </option>
          ))}
        </Select>
        <FieldError name="venueId" />
      </Label>
      <Label id="name-ro">
        {t("nameRo")}
        <Input name="name.ro" lang="ro" defaultValue={ev?.name.ro} />
        <FieldError name="name.ro" />
      </Label>
      <Label id="name-en">
        {t("nameEn")}
        <Input name="name.en" lang="en" defaultValue={ev?.name.en} />
        <FieldError name="name.en" />
      </Label>
      <Label id="description-ro">
        {t("descriptionRo")}
        <Textarea name="description.ro" lang="ro" defaultValue={ev?.description.ro} />
      </Label>
      <Label id="description-en">
        {t("descriptionEn")}
        <Textarea name="description.en" lang="en" defaultValue={ev?.description.en} />
      </Label>
      <Label>
        {t("slug")}
        <Input name="slug" required autoCapitalize="none" spellCheck={false} defaultValue={ev?.slug} />
        <FieldError name="slug" />
      </Label>
      <Label>
        {t("timezone")}
        <Input name="timezone" defaultValue={tz} />
        <FieldError name="timezone" />
      </Label>
      <Label>
        {t("startsAt")}
        <Input name="startsAt" type="datetime-local" required defaultValue={ev ? toLocalInput(ev.starts_at, tz) : ""} />
        <FieldError name="startsAt" />
      </Label>
      <Label>
        {t("endsAt")}
        <Input name="endsAt" type="datetime-local" required defaultValue={ev ? toLocalInput(ev.ends_at, tz) : ""} />
        <FieldError name="endsAt" />
      </Label>
      {isNew && (
        <>
          <Label>
            {t("opens")}
            <Input name="opens" type="time" defaultValue="18:00" />
          </Label>
          <Label>
            {t("closes")}
            <Input name="closes" type="time" defaultValue="23:00" />
            <Hint>{t("closesHint")}</Hint>
          </Label>
        </>
      )}
    </div>
  );
}
