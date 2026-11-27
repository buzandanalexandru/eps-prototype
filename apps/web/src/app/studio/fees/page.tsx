import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { getFeePolicy, getPlatformFee } from "@/lib/organisers/domain";
import { FEE_POLICIES } from "@/lib/pricing";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { ActionForm } from "@/components/studio/action-form";
import { FeeExample } from "@/components/fee-example";
import { Card } from "@/components/ui/field";
import { setFeePolicyAction } from "../actions";

export const metadata = { title: "Fees" };

export default async function StudioFeesPage() {
  const s = await requireStudio();
  const t = await getTranslations("fees");
  const { policy, fee } = await studioTx(s, async (c) => ({ policy: await getFeePolicy(c), fee: await getPlatformFee(c, "RON") }));
  const owner = s.active.role === "owner";
  return (
    <section className="max-w-2xl space-y-6">
      <h1 className="text-2xl font-bold">{t("studioTitle")}</h1>
      <p className="text-sm text-muted-foreground">
        {t("studioLead")}{" "}
        <Link className="text-primary hover:underline" href="/fees">
          {t("publicLink")}
        </Link>
      </p>
      <Card>
        <h2 className="font-semibold">{t("organiserPolicy")}</h2>
        <p className="text-sm" data-testid="current-policy">
          {t("inForce")}: <strong>{t(`policies.${policy}.name`)}</strong>
        </p>
        {owner ? (
          <ActionForm action={setFeePolicyAction} submit={t("save")} testId="fee-policy-form">
            <fieldset className="space-y-2">
              <legend className="sr-only">{t("organiserPolicy")}</legend>
              {FEE_POLICIES.map((p) => (
                <label key={p} className="flex items-start gap-2 text-sm">
                  <input type="radio" name="policy" value={p} defaultChecked={policy === p} className="mt-1" />
                  <span>
                    <span className="font-medium">{t(`policies.${p}.name`)}</span>
                    <span className="block text-muted-foreground">{t(`policies.${p}.lead`)}</span>
                  </span>
                </label>
              ))}
            </fieldset>
          </ActionForm>
        ) : (
          <p className="text-sm text-muted-foreground" data-testid="owner-only-note">
            {t("ownerOnly")}
          </p>
        )}
        <p className="text-xs text-muted-foreground">{t("onlyUnpublished")}</p>
      </Card>
      <Card>
        <h2 className="font-semibold">{t("exampleTitle")}</h2>
        <FeeExample fee={fee} highlight={policy} />
        <p className="text-xs text-muted-foreground">{t("eventOverrideHint")}</p>
      </Card>
    </section>
  );
}
