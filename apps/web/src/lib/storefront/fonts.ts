// The curated font list (design D2.3). The families are self-hosted through next/font at build time
// (components/storefront/fonts.ts); the document stores only these ids.
export const FONTS = {
  inter: { label: "Inter", css: "var(--eps-ff-inter), system-ui, sans-serif" },
  montserrat: { label: "Montserrat", css: "var(--eps-ff-montserrat), system-ui, sans-serif" },
  poppins: { label: "Poppins", css: "var(--eps-ff-poppins), system-ui, sans-serif" },
  space_grotesk: { label: "Space Grotesk", css: "var(--eps-ff-space-grotesk), system-ui, sans-serif" },
  oswald: { label: "Oswald", css: "var(--eps-ff-oswald), Impact, sans-serif" },
  playfair: { label: "Playfair Display", css: "var(--eps-ff-playfair), Georgia, serif" },
  dm_serif: { label: "DM Serif Display", css: "var(--eps-ff-dm-serif), Georgia, serif" },
  lora: { label: "Lora", css: "var(--eps-ff-lora), Georgia, serif" },
  system: { label: "System", css: "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif" },
} as const;
export type FontId = keyof typeof FONTS;
export const FONT_IDS = Object.keys(FONTS) as [FontId, ...FontId[]];
