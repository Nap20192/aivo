import type { Theme } from "./types";

// Same remap as the prototype's themeVars: pick an accent family and point the
// design-system accent slots at it. "--red-50" doubles as the selected-row wash.
const ACCENTS: Record<string, [string, string, string, string, string]> = {
  "blood red": ["--red-600", "--red-700", "--red-800", "--red-100", "--red-50"],
  olive: ["--olive-600", "--olive-700", "--olive-800", "--olive-100", "--olive-100"],
  wine: ["--wine-600", "--wine-700", "--wine-800", "--wine-100", "--wine-100"],
  fire: ["--orange-600", "--orange-700", "--orange-800", "--orange-100", "--orange-100"],
};

export function themeVars(theme: Theme): Record<string, string> {
  const [solid, hover, active, soft, wash] =
    ACCENTS[theme.accent?.toLowerCase() ?? ""] ?? ACCENTS["blood red"];
  return {
    "--accent-solid": `var(${solid})`,
    "--accent-solid-hover": `var(${hover})`,
    "--accent-solid-active": `var(${active})`,
    "--accent-soft": `var(${soft})`,
    "--red-50": `var(${wash})`,
    "--text-link": `var(${hover})`,
    "--text-link-hover": `var(${active})`,
    ...(theme.css_vars ?? {}),
  };
}
