import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { listVenues, loadEventGraph } from "@/lib/catalogue/domain";
import { groupLineup } from "@/lib/catalogue/lineup";
import { eventReadiness } from "@/lib/catalogue/readiness";
import { closesNextDay } from "@/lib/catalogue/time";
import { nextYearOffset } from "@/lib/catalogue/editions";
import { listBrands } from "@/lib/organisers/domain";
import { FEE_POLICIES } from "@/lib/pricing";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { ActionForm, FieldError } from "@/components/studio/action-form";
import { EventFields } from "@/components/studio/event-fields";
import { EventNav } from "@/components/studio/event-nav";
import { ReadinessList } from "@/components/studio/readiness-list";
import { Card, Check, Hint, Input, Label, Select } from "@/components/ui/field";
import {
  addLineupAction,
  copyEditionAction,
  deleteEventAction,
  deleteLineupAction,
  saveEventAction,
  setDaysAction,
  setEventFeePolicyAction,
  setRulesAction,
} from "../../actions";

export const metadata = { title: "Event" };

export default async function EventPage({ params, searchParams }: { params: Promise<{ id: string }>; searchParams: Promise<{ copied?: string }> }) {
  const { id } = await params;
  const { copied } = await searchParams;
  const s = await requireStudio();
  const t = await getTranslations("events");
  const tf = await getTranslations("fees");
  const data = await studioTx(s, async (c) => {
    const g = await loadEventGraph(c, id);
    if (!g) return null;
    return { g, brands: await listBrands(c), venues: await listVenues(c) };
  });
  if (!data) notFound();
  const { g, brands, venues } = data;
  const ev = g.event;
  const problems = eventReadiness(g);
  const grouped = groupLineup(g.days, g.lineup);
  const owner = s.active.role === "owner";
  const defaultOffset = nextYearOffset(ev.starts_at, ev.timezone);
  const many = g.days.length > 14;
  return (
    <section className="space-y-6">
      <EventNav g={g} current="details" />
      {copied && (
        <p role="status" className="rounded-md border border-emerald-500/50 p-3 text-sm" data-testid="copied">
          {t("copiedLead")}
        </p>
      )}
      <ReadinessList problems={problems} />

      <Card>
        <h2 className="font-semibold">{t("details")}</h2>
        <ActionForm action={saveEventAction.bind(null, ev.id)} submit={t("save")} testId="event-form">
          <EventFields ev={ev} brands={brands} venues={venues} />
        </ActionForm>
      </Card>

      <Card id="days">
        <h2 className="font-semibold">{t("days")}</h2>
        <Hint>{t("daysHint")}</Hint>
        <ActionForm action={setDaysAction.bind(null, ev.id)} submit={t("saveDays")} testId="days-form">
          <details open={!many}>
            <summary className="cursor-pointer text-sm">{t("dayCount", { n: g.days.length })}</summary>
            <ul className="space-y-2 pt-2" data-testid="days">
              {g.days.map((d, i) => (
                <li key={d.id} className="flex flex-wrap items-end gap-2" data-testid="day">
                  <input type="hidden" name={`days.${i}.day`} value={d.day} />
                  <span className="w-28 text-sm font-medium">{d.day}</span>
                  <Label className="w-28">
                    {t("opens")}
                    <Input type="time" name={`days.${i}.opens`} defaultValue={d.opens} />
                  </Label>
                  <Label className="w-28">
                    {t("closes")}
                    <Input type="time" name={`days.${i}.closes`} defaultValue={d.closes} />
                  </Label>
                  {closesNextDay(d.opens, d.closes) && <span className="text-xs text-muted-foreground">{t("nextMorning")}</span>}
                  <Check name={`days.${i}.remove`} label={t("removeDay")} />
                </li>
              ))}
            </ul>
          </details>
          <div className="flex flex-wrap items-end gap-2">
            <Label className="w-40">
              {t("addDay")}
              <Input type="date" name="add.day" />
            </Label>
            <Label className="w-28">
              {t("opens")}
              <Input type="time" name="add.opens" defaultValue="18:00" />
            </Label>
            <Label className="w-28">
              {t("closes")}
              <Input type="time" name="add.closes" defaultValue="23:00" />
            </Label>
          </div>
          <FieldError name="days" />
        </ActionForm>
      </Card>

      <Card id="lineup">
        <h2 className="font-semibold">{t("lineup")}</h2>
        {g.lineup.length === 0 && <p className="text-sm text-muted-foreground">{t("noLineup")}</p>}
        <div className="space-y-3" data-testid="lineup">
          {grouped
            .filter((d) => d.stages.length)
            .map((d) => (
              <div key={d.day.id}>
                <h3 className="text-sm font-semibold">{d.day.day}</h3>
                {d.stages.map((st) => (
                  <div key={st.stage} className="pl-2">
                    <p className="text-xs font-medium text-muted-foreground">{st.stage || t("noStage")}</p>
                    <ul className="text-sm">
                      {st.slots.map((x) => (
                        <li key={x.id} className="flex flex-wrap items-center gap-2">
                          <span className="tabular-nums">{x.starts}</span> <span className="min-w-0 break-words">{x.artist}</span>
                          <ActionForm action={deleteLineupAction.bind(null, ev.id, x.id)} submit={t("remove")} variant="outline" className="inline-flex space-y-0" />
                        </li>
                      ))}
                    </ul>
                  </div>
                ))}
              </div>
            ))}
        </div>
        {g.days.length > 0 && (
          <ActionForm action={addLineupAction.bind(null, ev.id)} submit={t("addArtist")} testId="lineup-form">
            <div className="grid gap-2 sm:grid-cols-4">
              <Label>
                {t("day")}
                <Select name="dayId">
                  {g.days.map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.day}
                    </option>
                  ))}
                </Select>
              </Label>
              <Label>
                {t("artist")}
                <Input name="artist" />
                <FieldError name="artist" />
              </Label>
              <Label>
                {t("stage")}
                <Input name="stage" list="stages" />
                <datalist id="stages">
                  {(venues.find((v) => v.id === ev.venue_id)?.stages ?? []).map((x) => (
                    <option key={x} value={x} />
                  ))}
                </datalist>
              </Label>
              <Label>
                {t("startTime")}
                <Input name="starts" type="time" />
                <FieldError name="starts" />
              </Label>
            </div>
          </ActionForm>
        )}
      </Card>

      <Card id="rules">
        <h2 className="font-semibold">{t("rules")}</h2>
        <Hint>{t("rulesHint")}</Hint>
        <p className="text-sm" data-testid="rules-summary">
          {ev.min_age ? t("minAgeSummary", { n: ev.min_age }) : t("noMinAge")}
          {ev.consent_under_age ? ` · ${t("consentSummary", { n: ev.consent_under_age })}` : ""}
          {ev.max_per_account ? ` · ${t("maxAccountSummary", { n: ev.max_per_account })}` : ""}
        </p>
        <ActionForm action={setRulesAction.bind(null, ev.id)} submit={t("saveRules")} testId="rules-form">
          <div className="grid gap-3 sm:grid-cols-3">
            <Label>
              {t("minAge")}
              <Input name="minAge" inputMode="numeric" defaultValue={ev.min_age ?? ""} />
              <FieldError name="minAge" />
            </Label>
            <Label>
              {t("consentUnderAge")}
              <Input name="consentUnderAge" inputMode="numeric" defaultValue={ev.consent_under_age ?? ""} />
              <FieldError name="consentUnderAge" />
            </Label>
            <Label>
              {t("maxPerAccount")}
              <Input name="maxPerAccount" inputMode="numeric" defaultValue={ev.max_per_account ?? ""} />
              <FieldError name="maxPerAccount" />
            </Label>
          </div>
          <Hint>{t("maxPerOrderHint")}</Hint>
        </ActionForm>
      </Card>

      <Card id="fee-policy">
        <h2 className="font-semibold">{tf("eventOverride")}</h2>
        <p className="text-sm" data-testid="event-policy">
          {ev.fee_policy ? tf("overrideInForce", { policy: tf(`policies.${ev.fee_policy}.name`) }) : tf("usesOrganiser", { policy: tf(`policies.${g.organiserPolicy}.name`) })}
        </p>
        {owner ? (
          <ActionForm action={setEventFeePolicyAction.bind(null, ev.id)} submit={tf("save")} testId="event-policy-form">
            <Select name="policy" defaultValue={ev.fee_policy ?? ""} aria-label={tf("eventOverride")}>
              <option value="">{tf("noOverride")}</option>
              {FEE_POLICIES.map((p) => (
                <option key={p} value={p}>
                  {tf(`policies.${p}.name`)}
                </option>
              ))}
            </Select>
          </ActionForm>
        ) : (
          <p className="text-sm text-muted-foreground">{tf("ownerOnly")}</p>
        )}
      </Card>

      <Card id="edition">
        <h2 className="font-semibold">{t("copyTitle")}</h2>
        <Hint>{t("copyLead")}</Hint>
        <ActionForm action={copyEditionAction.bind(null, ev.id)} submit={t("copy")} testId="copy-form">
          <Label className="w-48">
            {t("offsetDays")}
            <Input name="offsetDays" inputMode="numeric" defaultValue={defaultOffset} />
            <FieldError name="offsetDays" />
          </Label>
        </ActionForm>
      </Card>

      {ev.status === "draft" && (
        <Card>
          <h2 className="font-semibold">{t("deleteTitle")}</h2>
          <ActionForm action={deleteEventAction.bind(null, ev.id)} submit={t("delete")} variant="destructive" />
        </Card>
      )}
    </section>
  );
}
