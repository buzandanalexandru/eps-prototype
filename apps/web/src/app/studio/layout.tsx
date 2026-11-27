import { redirect } from "next/navigation";
import { currentSession } from "@/lib/auth/session";

export default async function StudioLayout({ children }: { children: React.ReactNode }) {
  const session = await currentSession();
  if (!session) redirect("/sign-in?next=%2Fstudio");
  return <>{children}</>;
}
