// The curated storefront fonts (lib/storefront/fonts), self-hosted by next/font at build time: no
// request leaves for a font service when a fan opens a shop. latin-ext covers ă, â, î, ș and ț.
import { DM_Serif_Display, Inter, Lora, Montserrat, Oswald, Playfair_Display, Poppins, Space_Grotesk } from "next/font/google";

const inter = Inter({ subsets: ["latin", "latin-ext"], variable: "--eps-ff-inter", display: "swap" });
const montserrat = Montserrat({ subsets: ["latin", "latin-ext"], variable: "--eps-ff-montserrat", display: "swap" });
const poppins = Poppins({ subsets: ["latin", "latin-ext"], weight: ["400", "600", "700"], variable: "--eps-ff-poppins", display: "swap" });
const spaceGrotesk = Space_Grotesk({ subsets: ["latin", "latin-ext"], variable: "--eps-ff-space-grotesk", display: "swap" });
const oswald = Oswald({ subsets: ["latin", "latin-ext"], variable: "--eps-ff-oswald", display: "swap" });
const playfair = Playfair_Display({ subsets: ["latin", "latin-ext"], variable: "--eps-ff-playfair", display: "swap" });
const dmSerif = DM_Serif_Display({ subsets: ["latin", "latin-ext"], weight: "400", variable: "--eps-ff-dm-serif", display: "swap" });
const lora = Lora({ subsets: ["latin", "latin-ext"], variable: "--eps-ff-lora", display: "swap" });

/** Class names that define every curated family's variable; browsers fetch only the fonts in use. */
export const shopFontClass = [inter, montserrat, poppins, spaceGrotesk, oswald, playfair, dmSerif, lora].map((f) => f.variable).join(" ");
