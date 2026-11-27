import { getLocale, getTranslations } from "next-intl/server";
import { listBrands, listInvitations, listMembers } from "@/lib/organisers/domain";
import { listEvents } from "@/lib/catalogue/domain";
import { ROLES } from "@/lib/organisers/roles";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { lt } from "@/lib/studio/text";
import { ActionForm, FieldError } from "@/components/studio/action-form";
import { InviteLink } from "@/components/studio/invite-link";
import { Card, Hint, Input, Label, Select } from "@/components/ui/field";
import { changeRoleAction, inviteAction, removeMemberAction, revokeInvitationAction } from "../actions";

export const metadata = { title: "Team" };

export default async function TeamPage() {
  const s = await requireStudio();
  const t = await getTranslations("team");
  const locale = await getLocale();
  const { members, invitations, brands, events } = await studioTx(s, async (c) => ({
    members: await listMembers(c),
    invitations: await listInvitations(c),
    brands: await listBrands(c),
    events: await listEvents(c),
  }));
  const owner = s.active.role === "owner";
  const scopes = [
    { value: "organiser", label: t("scope.organiser") },
    ...brands.map((b) => ({ value: `brand:${b.id}`, label: t("scope.brand", { name: b.name }) })),
    ...events.map((e) => ({ value: `event:${e.id}`, label: t("scope.event", { name: lt(e.name, locale) || e.slug }) })),
  ];
  const scopeLabel = (type: string, id: string | null) => scopes.find((x) => x.value === (type === "organiser" ? "organiser" : `${type}:${id}`))?.label ?? type;
  return (
    <section className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("title")}</h1>
        <p className="text-sm text-muted-foreground">{t("lead")}</p>
        <p className="text-sm text-muted-foreground" data-testid="scope-note">
          {t("scopeNotEnforced")}
        </p>
        {!owner && <p className="mt-2 text-sm" data-testid="owner-only-note">{t("ownerOnly")}</p>}
      </div>
      <Card>
        <h2 className="font-semibold">{t("members")}</h2>
        <ul className="divide-y" data-testid="members">
          {members.map((m) => (
            <li key={m.user_id} className="space-y-2 py-3" data-testid="member">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <span className="min-w-0 break-all">
                  <span className="font-medium">{m.name}</span> <span className="text-sm text-muted-foreground">{m.email}</span>
                </span>
                <span className="text-sm" data-testid="member-role">
                  {t(`roles.${m.role}`)} · {scopeLabel(m.scope_type, m.scope_id)}
                </span>
              </div>
              {owner && (
                <div className="flex flex-wrap items-end gap-3">
                  <ActionForm action={changeRoleAction.bind(null, m.user_id)} submit={t("change")} variant="outline" className="flex flex-wrap items-end gap-2 space-y-0">
                    <Label className="min-w-32">
                      {t("role")}
                      <Select name="role" defaultValue={m.role}>
                        {ROLES.map((r) => (
                          <option key={r} value={r}>
                            {t(`roles.${r}`)}
                          </option>
                        ))}
                      </Select>
                    </Label>
                    <Label className="min-w-40">
                      {t("scopeLabel")}
                      <Select name="scope" defaultValue={m.scope_type === "organiser" ? "organiser" : `${m.scope_type}:${m.scope_id}`}>
                        {scopes.map((x) => (
                          <option key={x.value} value={x.value}>
                            {x.label}
                          </option>
                        ))}
                      </Select>
                    </Label>
                  </ActionForm>
                  <ActionForm action={removeMemberAction.bind(null, m.user_id)} submit={t("remove")} variant="destructive" />
                </div>
              )}
            </li>
          ))}
        </ul>
      </Card>
      <Card>
        <h2 className="font-semibold">{t("invitations")}</h2>
        {invitations.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("noInvitations")}</p>
        ) : (
          <ul className="divide-y" data-testid="invitations">
            {invitations.map((i) => (
              <li key={i.id} className="flex flex-wrap items-center justify-between gap-2 py-2" data-testid="invitation">
                <span className="min-w-0 break-all text-sm">
                  {i.email} · {t(`roles.${i.role}`)} · {scopeLabel(i.scope_type, i.scope_id)} · {t("pending")}
                </span>
                {owner && <ActionForm action={revokeInvitationAction.bind(null, i.id)} submit={t("revoke")} variant="outline" />}
              </li>
            ))}
          </ul>
        )}
        <h3 className="pt-2 font-medium">{t("invite")}</h3>
        <ActionForm action={inviteAction} submit={t("sendInvite")} testId="invite-form">
          <div className="grid gap-3 sm:grid-cols-3">
            <Label>
              {t("email")}
              <Input name="email" type="email" required />
              <FieldError name="email" />
            </Label>
            <Label>
              {t("role")}
              <Select name="role" defaultValue="admin">
                {ROLES.map((r) => (
                  <option key={r} value={r}>
                    {t(`roles.${r}`)}
                  </option>
                ))}
              </Select>
            </Label>
            <Label>
              {t("scopeLabel")}
              <ScopeSelect scopes={scopes} />
            </Label>
          </div>
          <Hint>{t("inviteHint")}</Hint>
          <InviteLink />
        </ActionForm>
      </Card>
    </section>
  );
}

function ScopeSelect({ scopes }: { scopes: { value: string; label: string }[] }) {
  // The invite action reads scopeType and scopeId; one select carries both as "type:id".
  return (
    <>
      <Select name="scopeCombined" defaultValue="organiser" data-scope-select>
        {scopes.map((x) => (
          <option key={x.value} value={x.value}>
            {x.label}
          </option>
        ))}
      </Select>
    </>
  );
}
