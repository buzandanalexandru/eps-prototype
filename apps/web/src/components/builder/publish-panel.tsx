"use client";
// Publish, history and preview links (design D2.1, D2.5; B5). Publish saves pending changes first,
// then publishes that exact draft revision; the server repeats every check.
import { useState } from "react";
import { useFormatter, useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/studio/copy-button";
import { createLinkAction, publishAction, restoreAction, revokeAllLinksAction, revokeLinkAction, type LinkView, type VersionView } from "@/app/studio/builder/actions";
import type { PublishProblem } from "@/lib/storefront/publish-checks";
import type { StorefrontDocument } from "@/lib/storefront/schema";

type Published = { number: number; live: number | null; versions: VersionView[] };

export function PublishPanel(props: {
  storefrontId: string;
  problems: PublishProblem[];
  doc: StorefrontDocument;
  getRevision: () => number;
  flush: () => Promise<{ ok: boolean; code?: string }>;
  saving: boolean;
  canDesign: boolean;
  live: { number: number; publishedAt: string } | null;
  versions: VersionView[];
  initialLinks: LinkView[];
  onGoTo: (p: PublishProblem) => void;
  onPublished: (r: Published) => void;
  onRestored: (r: Published & { revision: number; document: StorefrontDocument }) => void;
}) {
  const t = useTranslations("builder");
  const te = useTranslations("errors");
  const f = useFormatter();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<{ kind: "ok" | "error"; text: string } | null>(null);
  const [serverProblems, setServerProblems] = useState<PublishProblem[] | null>(null);
  const [restoring, setRestoring] = useState<number | null>(null);
  const [links, setLinks] = useState(props.initialLinks);
  const [fresh, setFresh] = useState<string | null>(null);
  const problems = serverProblems ?? props.problems;

  const tr = useTranslations("readiness");
  const problemText = (p: PublishProblem) =>
    t(`problems.${p.code}`, {
      locale: p.locale ? p.locale.toUpperCase() : "",
      field: p.path.split(".").slice(-2).join(" "),
      detail: p.detail ?? "",
      // Step 3's gate: the event's readiness problem, the pool and its numbers.
      event: p.event ?? "",
      reason: p.code === "event_not_ready" && p.detail && tr.has(`codes.${p.detail}`) ? tr(`codes.${p.detail}`, { detail: p.reasonDetail ?? "" }) : (p.detail ?? ""),
      pool: p.pool ?? "",
      used: p.used ?? 0,
      capacity: p.capacity ?? 0,
    });

  const publish = async () => {
    setBusy(true);
    setMessage(null);
    setServerProblems(null);
    const saved = await props.flush();
    if (!saved.ok) {
      setBusy(false);
      // The draft could not be saved: say why (a conflict, or the save error and where to fix it).
      setMessage({ kind: "error", text: saved.code === "draft_conflict" ? te("draft_conflict") : t("saveFirst", { reason: te.has(saved.code ?? "") ? te(saved.code!) : te("unexpected") }) });
      return;
    }
    const r = await publishAction(props.storefrontId, props.getRevision());
    setBusy(false);
    if (r.ok) {
      props.onPublished(r.data);
      setMessage({ kind: "ok", text: t("published", { n: r.data.number }) });
    } else if (r.code === "publish_blocked") {
      setServerProblems(r.problems ?? []);
    } else setMessage({ kind: "error", text: te.has(r.code) ? te(r.code) : te("unexpected") });
  };

  const restore = async (n: number) => {
    setBusy(true);
    setMessage(null);
    const saved = await props.flush();
    if (!saved.ok) {
      // Never restore over work that could not be saved: the organiser decides first.
      setBusy(false);
      setRestoring(null);
      setMessage({ kind: "error", text: saved.code === "draft_conflict" ? te("draft_conflict") : t("saveFirst", { reason: te.has(saved.code ?? "") ? te(saved.code!) : te("unexpected") }) });
      return;
    }
    const r = await restoreAction(props.storefrontId, n, props.getRevision());
    setBusy(false);
    setRestoring(null);
    if (r.ok) {
      props.onRestored(r.data);
      setMessage({ kind: "ok", text: t("restored", { n: r.data.number, from: n }) });
    } else if (r.code === "publish_blocked") setServerProblems(r.problems ?? []);
    else setMessage({ kind: "error", text: te.has(r.code) ? te(r.code) : te("unexpected") });
  };

  return (
    <div className="space-y-5" data-testid="publish-tab">
      <section className="space-y-2">
        <h2 className="font-semibold">{t("publishTitle")}</h2>
        {problems.length === 0 ? (
          <p className="text-sm text-emerald-700 dark:text-emerald-400" data-testid="no-problems">
            {t("readyToPublish")}
          </p>
        ) : (
          <ul className="space-y-1 text-sm" data-testid="problems">
            {problems.slice(0, 50).map((p, i) => (
              <li key={i}>
                {p.href ? (
                  // Catalogue problems are fixed in Studio's event screens.
                  <a href={p.href} className="text-destructive underline" data-testid="problem" data-code={p.code} data-path={p.path}>
                    {problemText(p)}
                  </a>
                ) : (
                  <button type="button" className="text-left text-destructive underline" onClick={() => props.onGoTo(p)} data-testid="problem" data-code={p.code} data-path={p.path}>
                    {problemText(p)}
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
        {props.canDesign && (
          <Button type="button" onClick={publish} disabled={busy || props.problems.length > 0} data-testid="publish">
            {busy ? t("publishing") : t("publish")}
          </Button>
        )}
        {message && (
          <p role="status" className={`text-sm ${message.kind === "ok" ? "text-emerald-700 dark:text-emerald-400" : "text-destructive"}`} data-testid="publish-message">
            {message.text}
          </p>
        )}
      </section>

      <section className="space-y-2">
        <h2 className="font-semibold">{t("history")}</h2>
        {props.versions.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("noVersions")}</p>
        ) : (
          <ol className="space-y-1 text-sm" data-testid="versions">
            {props.versions.map((v) => (
              <li key={v.number} className="flex flex-wrap items-center gap-2 rounded-md border p-2" data-testid="version" data-number={v.number}>
                <span className="font-semibold">v{v.number}</span>
                {props.live?.number === v.number && <span className="rounded-full bg-emerald-100 px-2 text-xs text-emerald-900">{t("liveBadge")}</span>}
                <span className="text-muted-foreground">
                  {f.dateTime(new Date(v.publishedAt), { dateStyle: "medium", timeStyle: "short" })}
                  {v.author ? ` · ${v.author}` : ""}
                </span>
                {v.restoredFrom && <span className="text-xs text-muted-foreground">{t("restoredFrom", { n: v.restoredFrom })}</span>}
                {props.canDesign && props.live?.number !== v.number && (
                  <span className="ml-auto">
                    {restoring === v.number ? (
                      <span role="alertdialog" aria-label={t("restore")} className="flex flex-wrap items-center gap-1 text-xs">
                        {t("restoreConfirm")}
                        <Button type="button" size="xs" disabled={busy} onClick={() => restore(v.number)} data-testid="confirm-restore">
                          {t("confirm")}
                        </Button>
                        <Button type="button" size="xs" variant="ghost" onClick={() => setRestoring(null)}>
                          {t("cancel")}
                        </Button>
                      </span>
                    ) : (
                      <Button type="button" size="xs" variant="outline" onClick={() => setRestoring(v.number)} data-testid="restore">
                        {t("restore")}
                      </Button>
                    )}
                  </span>
                )}
              </li>
            ))}
          </ol>
        )}
      </section>

      <section className="space-y-2">
        <h2 className="font-semibold">{t("previewLinks")}</h2>
        <p className="text-sm text-muted-foreground">{t("previewLinksLead")}</p>
        {props.canDesign && (
          <Button
            type="button"
            size="sm"
            variant="outline"
            data-testid="create-link"
            onClick={async () => {
              const saved = await props.flush();
              if (!saved.ok) {
                setMessage({ kind: "error", text: saved.code === "draft_conflict" ? te("draft_conflict") : t("saveFirst", { reason: te.has(saved.code ?? "") ? te(saved.code!) : te("unexpected") }) });
                return;
              }
              const r = await createLinkAction(props.storefrontId);
              if (r.ok) {
                setFresh(`${window.location.origin}${r.data.path}`);
                setLinks(r.data.links);
              } else setMessage({ kind: "error", text: te.has(r.code) ? te(r.code) : te("unexpected") });
            }}
          >
            {t("createLink")}
          </Button>
        )}
        {fresh && (
          <div className="space-y-1 rounded-md border border-emerald-500/50 p-2 text-sm" data-testid="fresh-link">
            <p>{t("linkOnce")}</p>
            <div className="flex flex-wrap items-center gap-2">
              <code className="min-w-0 break-all text-xs" data-testid="fresh-link-url">
                {fresh}
              </code>
              <CopyButton value={fresh} />
            </div>
          </div>
        )}
        {props.canDesign && links.some((l) => !l.revoked && !l.expired) && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            data-testid="revoke-all-links"
            onClick={async () => {
              const r = await revokeAllLinksAction(props.storefrontId);
              if (r.ok) setLinks(r.data);
            }}
          >
            {t("revokeAll")}
          </Button>
        )}
        <ul className="space-y-1 text-sm" data-testid="links">
          {links.map((l) => (
            <li key={l.id} className="flex flex-wrap items-center gap-2" data-testid="link-row">
              <span>{f.dateTime(new Date(l.createdAt), { dateStyle: "medium", timeStyle: "short" })}</span>
              <span className="text-muted-foreground">{l.revoked ? t("linkRevoked") : l.expired ? t("linkExpired") : t("linkExpires", { date: f.dateTime(new Date(l.expiresAt), { dateStyle: "medium" }) })}</span>
              {props.canDesign && !l.revoked && !l.expired && (
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  data-testid="revoke-link"
                  onClick={async () => {
                    const r = await revokeLinkAction(props.storefrontId, l.id);
                    if (r.ok) setLinks(r.data);
                  }}
                >
                  {t("revoke")}
                </Button>
              )}
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
