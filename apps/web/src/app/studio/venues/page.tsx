import { getTranslations } from "next-intl/server";
import { listVenues, type VenueRow } from "@/lib/catalogue/domain";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { ActionForm, FieldError } from "@/components/studio/action-form";
import { Card, Hint, Input, Label } from "@/components/ui/field";
import { saveVenueAction } from "../actions";

export const metadata = { title: "Venues" };

async function VenueFields({ v }: { v?: VenueRow }) {
  const t = await getTranslations("venues");
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <Label>
        {t("name")}
        <Input name="name" required defaultValue={v?.name} />
        <FieldError name="name" />
      </Label>
      <Label>
        {t("address")}
        <Input name="address" defaultValue={v?.address} />
      </Label>
      <Label>
        {t("lat")}
        <Input name="lat" inputMode="decimal" defaultValue={v?.lat ?? ""} />
        <FieldError name="lat" />
      </Label>
      <Label>
        {t("lng")}
        <Input name="lng" inputMode="decimal" defaultValue={v?.lng ?? ""} />
        <FieldError name="lng" />
      </Label>
      <Label className="sm:col-span-2">
        {t("stages")}
        <Input name="stages" defaultValue={v?.stages.join(", ")} />
        <Hint>{t("stagesHint")}</Hint>
      </Label>
    </div>
  );
}

export default async function VenuesPage() {
  const s = await requireStudio();
  const t = await getTranslations("venues");
  const venues = await studioTx(s, (c) => listVenues(c));
  return (
    <section className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("title")}</h1>
        <p className="text-sm text-muted-foreground">{t("lead")}</p>
      </div>
      <ul className="space-y-3" data-testid="venue-list">
        {venues.map((v) => (
          <li key={v.id}>
            <Card>
              <details>
                <summary className="cursor-pointer">
                  <span className="font-semibold">{v.name}</span>{" "}
                  <span className="text-sm text-muted-foreground">{t("usedBy", { n: v.events })}</span>
                </summary>
                <ActionForm action={saveVenueAction.bind(null, v.id)} submit={t("save")} className="pt-3">
                  <VenueFields v={v} />
                </ActionForm>
              </details>
            </Card>
          </li>
        ))}
      </ul>
      <Card>
        <h2 className="font-semibold">{t("add")}</h2>
        <ActionForm action={saveVenueAction.bind(null, null)} submit={t("create")} testId="new-venue-form">
          <VenueFields />
        </ActionForm>
      </Card>
    </section>
  );
}
