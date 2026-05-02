const FALLBACK_THEME_COLORS: Record<string, string> = {
	"--color-component-background": "#282a2c",
	"--color-normal-text": "#ffffff",
	"--color-complement-text": "#888787",
};

export function getThemeColor(varName: string): string {
	if (typeof document === "undefined") {
		return FALLBACK_THEME_COLORS[varName] ?? "";
	}

	const color = getComputedStyle(document.documentElement)
		.getPropertyValue(varName)
		.trim();

	return color || FALLBACK_THEME_COLORS[varName] || "";
}
