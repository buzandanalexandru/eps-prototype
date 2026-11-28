// Slot grids (client-safe: no node imports), shared by the compiler and the shop's picker.
import { slotSpan } from "../catalogue/schema";
import { minutesOf } from "../catalogue/time";

/** "HH:MM" plus minutes, wrapping at midnight. */
function addMinutes(time: string, mins: number): string {
  const t = (minutesOf(time) + mins) % 1440;
  return `${String(Math.floor(t / 60)).padStart(2, "0")}:${String(t % 60).padStart(2, "0")}`;
}

/** The start times of a slot grid, first to last (one time grid per slot pool; readiness rule). */
export function slotStarts(grid: { slotMinutes: number; firstStart: string; lastStart: string }): string[] {
  const n = slotSpan(grid.firstStart, grid.lastStart) / grid.slotMinutes + 1;
  return Array.from({ length: n }, (_, i) => addMinutes(grid.firstStart, i * grid.slotMinutes));
}
