import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { listVenues } from "@/lib/catalogue/domain";
import { listBrands } from "@/lib/organisers/domain";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { ActionForm } from "@/components/studio/action-form";
import { EventFields } from "@/components/studio/event-fields";
import { Card } from "@/components/ui/field";
import { saveEventAction } from "../../actions";

export const metadata = { title: "New event" };

export default async function NewEventPage() {
  const s = await requireStudio();
  const t = await getTranslations("events");
  const { brands, venues } = await studioTx(s, async (c) => ({ brands: await listBrands(c), venues: await listVenues(c) }));
  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-bold">{t("new")}</h1>
      {venues.length === 0 && (
        <p className="text-sm text-muted-foreground">
          {t("venueFirst")}{" "}
          <Link href="/studio/venues" className="text-primary hover:underline">
            {t("addVenue")}
          </Link>
        </p>
      )}
      <Card>
        <ActionForm action={saveEventAction.bind(null, null)} submit={t("create")} testId="new-event-form">
          <EventFields brands={brands} venues={venues} isNew />
        </ActionForm>
      </Card>
    </section>
  );
}
