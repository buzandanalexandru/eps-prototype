import { getTranslations } from "next-intl/server";
import { getStudio } from "@/lib/studio/context";
import { ActionForm } from "@/components/studio/action-form";
import { acceptInvitationAction } from "../../../actions";

export const metadata = { title: "Invitation" };

export default async function InvitePage({ params }: { params: Promise<{ org: string; token: string }> }) {
  const { org, token } = await params;
  const s = await getStudio();
  const t = await getTranslations("team");
  return (
    <section className="max-w-xl space-y-4">
      <h1 className="text-2xl font-bold">{t("acceptTitle")}</h1>
      <p className="text-muted-foreground">{t("acceptLead", { email: s?.user.email ?? "" })}</p>
      <ActionForm action={acceptInvitationAction.bind(null, org, token)} submit={t("accept")} testId="accept-form" />
    </section>
  );
}
