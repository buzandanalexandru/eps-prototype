import { deflateSync } from "node:zlib";
import { expect, type Page } from "@playwright/test";

/** A small PNG made of a coloured square on transparency (for the logo upload and its palette). */
export function colourPng(rgb: [number, number, number], size = 16): Buffer {
  const crcTable = Array.from({ length: 256 }, (_, n) => {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    return c >>> 0;
  });
  const crc = (b: Buffer) => {
    let c = 0xffffffff;
    for (const x of b) c = crcTable[(c ^ x) & 0xff] ^ (c >>> 8);
    return (c ^ 0xffffffff) >>> 0;
  };
  const chunk = (type: string, data: Buffer) => {
    const len = Buffer.alloc(4);
    len.writeUInt32BE(data.length);
    const td = Buffer.concat([Buffer.from(type), data]);
    const c = Buffer.alloc(4);
    c.writeUInt32BE(crc(td));
    return Buffer.concat([len, td, c]);
  };
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(size, 0);
  ihdr.writeUInt32BE(size, 4);
  ihdr[8] = 8;
  ihdr[9] = 6;
  const rows: number[] = [];
  for (let y = 0; y < size; y++) {
    rows.push(0);
    for (let x = 0; x < size; x++) rows.push(...rgb, 255);
  }
  return Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), chunk("IHDR", ihdr), chunk("IDAT", deflateSync(Buffer.from(rows))), chunk("IEND", Buffer.alloc(0))]);
}

export const frame = (page: Page) => page.frameLocator("[data-testid=preview-frame]");

/** Opens the builder and waits until the frame has rendered a draft sent by the builder. */
export async function openBuilder(page: Page, query = "") {
  await page.goto(`/studio/builder${query}`);
  await expect(page.getByTestId("builder")).toBeVisible();
  await expect(frame(page).getByTestId("storefront")).toBeVisible();
  await expect(page.getByTestId("builder")).not.toHaveAttribute("data-rendered", "-1");
}

/** Waits for autosave to settle. */
export async function saved(page: Page) {
  await expect(page.getByTestId("save-state")).toHaveText(/All changes saved|Toate modificările sunt salvate/, { timeout: 10_000 });
}

export async function sectionTypes(page: Page): Promise<string[]> {
  return page.getByTestId("section-row").evaluateAll((els) => els.map((e) => e.getAttribute("data-section-type") ?? ""));
}

export async function frameTypes(page: Page): Promise<string[]> {
  return frame(page).locator("[data-eps-type]").evaluateAll((els) => els.map((e) => e.getAttribute("data-eps-type") ?? ""));
}

/** On a phone the builder shows one pane at a time. */
export async function showPane(page: Page, pane: "structure" | "preview" | "settings") {
  const tab = page.getByTestId(`pane-${pane}`);
  if (await tab.isVisible()) await tab.click();
}
