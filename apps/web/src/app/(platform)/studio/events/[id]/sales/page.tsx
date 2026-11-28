// Studio's sale settings screen (step 5, E1, E4): the event's sale moments, each with its opening,
// waiting room, the live admission dial, the shopping limit, pause, holds, caps, bot defence and
// the OTP switch; the live waiting-room figures; and every saved version. The settings live in
// the core's sales schema (the contract's /v1/organiser/... operations).
import { notFound } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { loadEventGraph } from "@/lib/catalogue/domain";
import { toLocalInput } from "@/lib/catalogue/time";
import { organiserCore } from "@/lib/checkout/studio-core";
import type { components } from "@/lib/core/schema";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { EventNav } from "@/components/studio/event-nav";
import { Card, Check, Hint, Input, Label } from "@/components/ui/field";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/studio/copy-button";
import { RateDial } from "@/components/studio/rate-dial";
import { createSaleAction, deleteSaleAction, pauseSaleAction, saveSaleAction } from "./actions";

export const metadata = { title: "Sale moments" };
export const dynamic = "force-dynamic";

type Sale = components["schemas"]["SaleView"];
type Settings = components["schemas"]["SaleSettings"];
type Search = Promise<Record<string, string | undefined>>;

function defaults(): Settings {
  const at = new Date(Date.now() + 24 * 3600_000);
  at.setUTCMinutes(0, 0, 0);
  return { opensAt: at.toISOString(), prequeueMinutes: 30, queueOn: true, ratePerMinute: 2000, activeLimit: 1500, paused: false, holdMinutes: 8, capPerOrder: 4, capPerPerson: 6, botDefence: true, otpOn: false, otpThreshold: 4, accountsOnly: false };
}

async function Fields({ s, tz, locked }: { s: Settings; tz: string; locked: boolean }) {
  const t = await getTranslations("sales");
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <input type="hidden" name="tz" value={tz} />
      <Label>
        {t("opensAt", { tz })}
        <Input type="datetime-local" name="opensAt" defaultValue={toLocalInput(new Date(s.opensAt), tz)} readOnly={locked} aria-readonly={locked} data-testid="sale-opens-at" />
      </Label>
      <Label>
        {t("prequeueMinutes")}
        <Input type="number" name="prequeueMinutes" min={5} max={120} defaultValue={s.prequeueMinutes} readOnly={locked} data-testid="sale-prequeue" />
      </Label>
      {locked ? (
        <>
          {s.queueOn && <input type="hidden" name="queueOn" value="on" />}
          <p className="text-sm sm:col-span-2" data-testid="sale-locked">
            {t("queueOn")}: {s.queueOn ? t("yes") : t("no")} · {t("locked")}
          </p>
        </>
      ) : (
        <Check name="queueOn" label={t("queueOn")} defaultChecked={s.queueOn} className="sm:col-span-2" data-testid="sale-queue-on" />
      )}
      <Label className="sm:col-span-2">
        {t("rate")}
        <RateDial name="ratePerMinute" defaultValue={s.ratePerMinute} label={t("rate")} />
        <Hint>{t("rateHint")}</Hint>
      </Label>
      <Label>
        {t("activeLimit")}
        <Input type="number" name="activeLimit" min={1} max={100000} defaultValue={s.activeLimit} data-testid="sale-active-limit" />
      </Label>
      <Check name="paused" label={t("paused")} defaultChecked={s.paused} data-testid="sale-paused" />
      <Label>
        {t("holdMinutes")}
        <Input type="number" name="holdMinutes" defaultValue={s.holdMinutes} data-testid="sale-hold-minutes" />
      </Label>
      <Label>
        {t("capPerOrder")}
        <Input type="number" name="capPerOrder" defaultValue={s.capPerOrder} data-testid="sale-cap-order" />
      </Label>
      <Label>
        {t("capPerPerson")}
        <Input type="number" name="capPerPerson" defaultValue={s.capPerPerson} data-testid="sale-cap-person" />
      </Label>
      <Check name="botDefence" label={t("botDefence")} defaultChecked={s.botDefence} data-testid="sale-bot-defence" />
      <Check name="otpOn" label={t("otpOn")} defaultChecked={s.otpOn} data-testid="sale-otp-on" />
      <Label>
        {t("otpThreshold")}
        <Input type="number" name="otpThreshold" defaultValue={s.otpThreshold} data-testid="sale-otp-threshold" />
      </Label>
      <Check name="accountsOnly" label={t("accountsOnly")} defaultChecked={!!s.accountsOnly} data-testid="sale-accounts-only" />
    </div>
  );
}

export default async function SalesPage({ params, searchParams }: { params: Promise<{ id: string }>; searchParams: Search }) {
  const { id } = await params;
  const sp = await searchParams;
  const s = await requireStudio();
  const t = await getTranslations("sales");
  const locale = await getLocale();
  const g = await studioTx(s, (c) => loadEventGraph(c, id));
  if (!g) notFound();
  const tz = g.event.timezone;
  const r = await organiserCore<{ sales: Sale[] }>("GET", `/v1/organiser/events/${id}/sales`);
  const list = r.ok ? r.data.sales : [];
  const details = await Promise.all(list.map((x) => organiserCore<Sale>("GET", `/v1/organiser/sales/${x.id}`)));
  const admin = s.active.role === "owner" || s.active.role === "admin";
  const shopLocale = g.brand.languages.includes(locale as "ro" | "en") ? locale : g.brand.languages[0] ?? "ro";
  const when = new Intl.DateTimeFormat(locale === "ro" ? "ro-RO" : "en-GB", { dateStyle: "medium", timeStyle: "medium", timeZone: tz });
  const errorText =
    sp.error === "EPS-SALE-INVALID" && sp.field
      ? t("errorRange", { field: sp.field, min: sp.min ?? "", max: sp.max ?? "" })
      : sp.error === "EPS-SALE-LOCKED"
        ? t("errorLocked")
        : sp.error === "EPS-SALE-NOT-FOUND"
          ? t("notPublished")
          : sp.error === "EPS-SALE-STALE"
            ? t("errorStale")
            : (sp.error ?? "");
  return (
    <section className="space-y-6">
      <EventNav g={g} current="sales" />
      <h2 className="text-xl font-semibold">{t("title")}</h2>
      <p className="text-sm text-muted-foreground">{t("lead")}</p>
      {s.active.is_sample && (
        <p role="note" className="rounded-md border border-dashed p-2 text-sm" data-testid="sale-sample-note">
          {t("sampleShared")}
        </p>
      )}
      {sp.deleted && (
        <p role="status" className="text-sm font-semibold" data-testid="sale-deleted">
          {t("deleted")}
        </p>
      )}
      {sp.saved && sp.v && (
        <p role="status" className="text-sm font-semibold" data-testid="sale-saved">
          {t("saved", { version: sp.v })}
        </p>
      )}
      {sp.error && (
        <p role="alert" className="text-sm font-semibold text-destructive" data-testid="sale-error">
          {t("error", { message: errorText })}
        </p>
      )}
      {!r.ok && <p role="alert">{r.code}</p>}
      {list.length === 0 && r.ok && <p className="text-sm">{t("none")}</p>}
      <ul className="space-y-4" data-testid="sale-list">
        {list.map((x, i) => {
          const d = details[i].ok ? details[i].data : x;
          const locked = d.phase !== "before";
          const link = `/s/${g.brand.slug}/${shopLocale}/queue?sale=${d.id}`;
          return (
            <li key={d.id} id={`sale-${d.id}`} data-testid="sale" data-sale-id={d.id} data-version={d.version}>
              <Card>
                <div className="flex flex-wrap items-baseline justify-between gap-2">
                  <h3 className="font-semibold" data-testid="sale-name">
                    {d.name}
                  </h3>
                  <span className="text-sm" data-testid="sale-version">
                    {t("version", { version: d.version })} · {t("phase")}: {t(`phase_${d.phase}`)}
                  </span>
                </div>
                {d.queue && (
                  <dl className="grid grid-cols-2 gap-2 text-sm sm:grid-cols-5" data-testid="sale-queue">
                    {(
                      [
                        ["queueState", d.queue.state || "-"],
                        ["prequeue", d.queue.prequeue],
                        ["line", d.queue.line + d.queue.slow],
                        ["admitted", d.queue.admitted],
                        ["active", d.queue.active],
                      ] as const
                    ).map(([k, v]) => (
                      <div key={k}>
                        <dt className="text-xs text-muted-foreground">{t(k)}</dt>
                        <dd className="font-semibold" data-testid={`sale-queue-${k}`}>
                          {String(v)}
                        </dd>
                      </div>
                    ))}
                  </dl>
                )}
                {d.queue?.seed && (
                  <p className="text-xs break-all">
                    {t("seed")}: <code>{d.queue.seed}</code>
                  </p>
                )}
                <div className="flex flex-wrap items-center gap-2 text-sm">
                  <span>{t("link")}:</span>
                  <a href={link} className="break-all underline" data-testid="sale-link">
                    {link}
                  </a>
                  <CopyButton value={link} />
                </div>
                {admin && (
                  <>
                    <div className="flex flex-wrap gap-2">
                      <form action={pauseSaleAction.bind(null, id, d.id, !d.settings.paused)}>
                        <Button type="submit" variant="outline" data-testid="sale-pause-toggle">
                          {d.settings.paused ? t("resume") : t("pauseNow")}
                        </Button>
                      </form>
                      {!locked && (
                        <form action={deleteSaleAction.bind(null, id, d.id)}>
                          <Button type="submit" variant="destructive" data-testid="sale-delete">
                            {t("delete")}
                          </Button>
                        </form>
                      )}
                    </div>
                    <form action={saveSaleAction.bind(null, id, d.id)} className="space-y-3" data-testid="sale-form">
                      <input type="hidden" name="baseVersion" value={d.version} />
                      <Fields s={d.settings} tz={tz} locked={locked} />
                      <Button type="submit" data-testid="sale-save">
                        {t("save")}
                      </Button>
                    </form>
                  </>
                )}
                {d.history && d.history.length > 0 && (
                  <details>
                    <summary className="cursor-pointer text-sm">{t("history")}</summary>
                    <div className="overflow-x-auto">
                      <table className="w-full text-left text-xs" data-testid="sale-history">
                        <thead>
                          <tr>
                            <th className="pr-2">#</th>
                            <th className="pr-2">{t("historyWhen")}</th>
                            <th className="pr-2">{t("historyBy")}</th>
                            <th className="pr-2">{t("historyRate")}</th>
                            <th>{t("historyPaused")}</th>
                          </tr>
                        </thead>
                        <tbody>
                          {d.history.map((h) => (
                            <tr key={h.version} data-testid="sale-history-row">
                              <td className="pr-2">{h.version}</td>
                              <td className="pr-2">{when.format(new Date(h.at))}</td>
                              <td className="pr-2 break-all">{h.by === s.user.id ? s.user.email : h.by.slice(0, 8)}</td>
                              <td className="pr-2">{h.settings.ratePerMinute}</td>
                              <td>{h.settings.paused ? t("yes") : t("no")}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </details>
                )}
              </Card>
            </li>
          );
        })}
      </ul>
      {admin && (
        <Card>
          <h3 className="font-semibold">{t("create")}</h3>
          <form action={createSaleAction.bind(null, id)} className="space-y-3" data-testid="new-sale-form">
            <Label>
              {t("name")}
              <Input name="name" required maxLength={80} data-testid="sale-new-name" />
            </Label>
            <Fields s={defaults()} tz={tz} locked={false} />
            <Button type="submit" data-testid="sale-create">
              {t("create")}
            </Button>
          </form>
        </Card>
      )}
    </section>
  );
}
