// The door scanning page (G2): for the event organiser's owner, admins and box office. The page is
// kept by a service worker once the device is prepared, so it reloads and scans offline.
import { getLocale, getTranslations } from "next-intl/server";
import { notFound } from "next/navigation";
import { DoorApp } from "@/components/tickets/door-app";

export const dynamic = "force-dynamic";

export async function generateMetadata() {
  const t = await getTranslations("door");
  return { title: t("title"), robots: { index: false, follow: false } };
}

export default async function DoorPage({ params }: { params: Promise<{ event: string }> }) {
  const { event } = await params;
  if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(event)) notFound();
  const t = await getTranslations("door");
  const locale = (await getLocale()) === "en" ? "en" : "ro";
  return (
    <section className="mx-auto max-w-md space-y-3">
      <h1 className="text-2xl font-bold">{t("title")}</h1>
      <DoorApp eventId={event} locale={locale} />
    </section>
  );
}
