// Line-up grouping for Studio and storefronts: per day, per stage (alphabetical), in night order
// (a 01:00 set comes after a 23:00 set on the same night).
import type { DayRow, LineupRow } from "./domain";
import { nightOrder } from "./time";

export function groupLineup(days: DayRow[], slots: LineupRow[]) {
  return days.map((d) => {
    const mine = slots.filter((s) => s.day_id === d.id);
    const stages = [...new Set(mine.map((s) => s.stage))].sort((a, b) => a.localeCompare(b));
    return {
      day: d,
      stages: stages.map((stage) => ({
        stage,
        slots: mine
          .filter((s) => s.stage === stage)
          .sort((a, b) => nightOrder(a.starts, d.opens) - nightOrder(b.starts, d.opens) || a.sort - b.sort || a.artist.localeCompare(b.artist)),
      })),
    };
  });
}
