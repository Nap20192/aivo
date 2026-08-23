// Restaurant theme → CSS custom properties, the prototype's themeVars mapping.
// Accent keys are lowercased HERE, once — callers may pass "Blood red",
// "blood red", or anything else (unknown falls back to blood red).

/** Display names for accent pickers, in canonical order. */
export const ACCENT_NAMES = ["Blood red", "Olive", "Wine", "Fire"] as const;
export type AccentName = (typeof ACCENT_NAMES)[number];

// [solid, hover, active, soft, wash] token names per accent family.
const ACCENT_MAP: Record<string, [string, string, string, string, string]> = {
  "blood red": ["--red-600", "--red-700", "--red-800", "--red-100", "--red-50"],
  olive: ["--olive-600", "--olive-700", "--olive-800", "--olive-100", "--olive-100"],
  wine: ["--wine-600", "--wine-700", "--wine-800", "--wine-100", "--wine-100"],
  fire: ["--orange-600", "--orange-700", "--orange-800", "--orange-100", "--orange-100"],
};

export function themeVars(theme: {
  accent?: string | null;
  css_vars?: Record<string, string> | null;
}): Record<string, string> {
  const [solid, hover, active, soft, wash] =
    ACCENT_MAP[theme.accent?.toLowerCase() ?? ""] ?? ACCENT_MAP["blood red"];
  return {
    "--accent-solid": `var(${solid})`,
    "--accent-solid-hover": `var(${hover})`,
    "--accent-solid-active": `var(${active})`,
    "--accent-soft": `var(${soft})`,
    // The prototype reuses --red-50 as the selected-row wash slot.
    "--red-50": `var(${wash})`,
    "--text-link": `var(${hover})`,
    "--text-link-hover": `var(${active})`,
    ...(theme.css_vars ?? {}),
  };
}
