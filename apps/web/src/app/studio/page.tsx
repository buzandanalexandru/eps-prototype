import { getTranslations } from "next-intl/server";
import { Button } from "@/components/ui/button";
import { currentSession } from "@/lib/auth/session";
import { signOut } from "@/lib/session/actions";

export const metadata = { title: "Studio" };

export default async function StudioPage() {
  const session = await currentSession();
  const t = await getTranslations("studio");
  const tc = await getTranslations("common");
  return (
    <section className="space-y-4">
      <h1 className="text-2xl font-bold">{t("title")}</h1>
      <p data-testid="studio-user">{t("welcome", { name: session?.user.name || session?.user.email || "" })}</p>
      <p className="text-muted-foreground">{t("empty")}</p>
      <form action={signOut}>
        <Button type="submit" variant="outline">
          {tc("signOut")}
        </Button>
      </form>
    </section>
  );
}
