import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getStudio } from "@/lib/studio/context";
import { ActionForm, FieldError } from "@/components/studio/action-form";
import { BrandFields } from "@/components/studio/brand-fields";
import { Hint, Input, Label } from "@/components/ui/field";
import { completeOnboardingAction, createOrganiserAction } from "../actions";

export const metadata = { title: "Onboarding" };

export default async function OnboardingPage() {
  const s = await getStudio();
  if (!s) redirect("/sign-in?next=%2Fstudio%2Fonboarding");
  const t = await getTranslations("onboarding");
  const pending = s.organisers.find((o) => o.onboarding_state === "brand" && o.role === "owner");
  if (!pending && s.organisers.some((o) => o.onboarding_state === "done")) redirect("/studio");
  const step = pending ? 2 : 1;
  return (
    <section className="max-w-xl space-y-6">
      <div className="space-y-1">
        <p className="text-sm text-muted-foreground" data-testid="onboarding-step">
          {t("step", { n: step, of: 2 })}
        </p>
        <h1 className="text-2xl font-bold">{step === 1 ? t("orgTitle") : t("brandTitle", { org: pending!.name })}</h1>
        <p className="text-muted-foreground">{step === 1 ? t("orgLead") : t("brandLead")}</p>
      </div>
      {step === 1 ? (
        <ActionForm action={createOrganiserAction} submit={t("continue")} testId="org-form">
          <Label>
            {t("orgName")}
            <Input name="name" required maxLength={120} autoComplete="organization" />
            <FieldError name="name" />
          </Label>
          <Label>
            {t("orgSlug")}
            <Input name="slug" required pattern="[a-z0-9-]{3,40}" autoCapitalize="none" spellCheck={false} />
            <Hint>{t("slugHint")}</Hint>
            <FieldError name="slug" />
          </Label>
        </ActionForm>
      ) : (
        <ActionForm action={completeOnboardingAction} submit={t("finish")} testId="brand-form">
          <BrandFields withTemplate />
        </ActionForm>
      )}
    </section>
  );
}
