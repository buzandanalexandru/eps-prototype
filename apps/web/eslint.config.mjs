import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  // All-in prices everywhere (step 3, design D3.7): storefront code formats money only through
  // components/storefront/all-in-price.tsx, which renders snapshot amounts with their fee line.
  {
    files: ["src/components/storefront/**/*.{ts,tsx}", "src/app/(shop)/**/*.{ts,tsx}", "src/lib/storefront/**/*.{ts,tsx}"],
    ignores: ["src/components/storefront/all-in-price.tsx"],
    rules: {
      "no-restricted-imports": [
        "error",
        {
          paths: [
            { name: "@/lib/pricing/money", importNames: ["formatMoney", "toMajorString"], message: "Show money through components/storefront/all-in-price.tsx (all-in prices only)." },
            { name: "@/lib/pricing", importNames: ["formatMoney", "toMajorString"], message: "Show money through components/storefront/all-in-price.tsx (all-in prices only)." },
            { name: "../pricing/money", importNames: ["formatMoney", "toMajorString"], message: "Show money through components/storefront/all-in-price.tsx (all-in prices only)." },
          ],
        },
      ],
      "no-restricted-syntax": [
        "error",
        {
          selector: "NewExpression[callee.object.name='Intl'][callee.property.name='NumberFormat']",
          message: "Show money through components/storefront/all-in-price.tsx (all-in prices only).",
        },
        {
          selector: "CallExpression[callee.property.name='toLocaleString']",
          message: "Show money through components/storefront/all-in-price.tsx (all-in prices only).",
        },
      ],
    },
  },
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
  ]),
]);

export default eslintConfig;
