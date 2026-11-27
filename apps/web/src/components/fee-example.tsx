import { getLocale, getTranslations } from "next-intl/server";
import { FEE_POLICIES, feeBreakdown, formatMoney, type FeePolicy, type PlatformFee } from "@/lib/pricing";

/** The three policies applied to one face price: what the fan pays and what the organiser receives. */
export async function FeeExample({ fee, faceMinor = 53900, highlight }: { fee: PlatformFee; faceMinor?: number; highlight?: FeePolicy }) {
  const t = await getTranslations("fees");
  const locale = await getLocale();
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-0 text-sm" data-testid="fee-example">
        <thead>
          <tr className="text-left text-muted-foreground">
            <th className="py-1 pr-2 font-medium">{t("policy")}</th>
            <th className="py-1 pr-2 font-medium">{t("fanPays")}</th>
            <th className="py-1 font-medium">{t("youReceive")}</th>
          </tr>
        </thead>
        <tbody>
          {FEE_POLICIES.map((p) => {
            const b = feeBreakdown(faceMinor, p, fee);
            return (
              <tr key={p} className={`border-t ${highlight === p ? "font-semibold" : ""}`} data-testid={`example-${p}`}>
                <td className="py-1 pr-2">{t(`policies.${p}.name`)}</td>
                <td className="py-1 pr-2 whitespace-nowrap" data-testid="fan-pays">
                  {formatMoney(b.allInMinor, "RON", locale)}
                </td>
                <td className="py-1 whitespace-nowrap" data-testid="you-receive">
                  {formatMoney(b.organiserNetMinor, "RON", locale)}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

export function percent(bps: number, locale: string): string {
  const s = `${Math.floor(bps / 100)}${bps % 100 ? (locale === "ro" ? "," : ".") + String(bps % 100).padStart(2, "0").replace(/0$/, "") : ""}`;
  return `${s}%`;
}
