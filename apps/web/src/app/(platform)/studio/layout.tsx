import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { StudioHeader } from "@/components/studio/studio-header";
import { getStudio } from "@/lib/studio/context";

export default async function StudioLayout({ children }: { children: React.ReactNode }) {
  const studio = await getStudio();
  if (!studio) redirect("/sign-in?next=%2Fstudio");
  if ((await headers()).get("x-eps-layout") === "bare") return <>{children}</>;
  return (
    <>
      <StudioHeader studio={studio} />
      {children}
    </>
  );
}
