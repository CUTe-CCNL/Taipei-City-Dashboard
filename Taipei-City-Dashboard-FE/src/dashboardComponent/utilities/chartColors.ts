const FALLBACK_COLOR = "#848c94";

type ChartDataPoint = {
	x: any;
	y: number;
};

type ThresholdRule = {
	min: number;
	max: number;
	color: string;
};

function parseThresholdRule(rule: string): ThresholdRule | null {
	const parts = rule.split(":");
	if (parts.length < 3) {
		return null;
	}

	const min = Number(parts[0]);
	const maxText = parts[1];
	const max = maxText === "" ? Infinity : Number(maxText);
	const color = parts.slice(2).join(":");

	if (!Number.isFinite(min) || !Number.isFinite(max) || !color) {
		return null;
	}

	return { min, max, color };
}

export function resolveChartColors(
	colorConfig: string[],
	data: ChartDataPoint[]
): string[] {
	if (!Array.isArray(colorConfig) || colorConfig.length === 0) {
		return data.map(() => FALLBACK_COLOR);
	}

	const mode = colorConfig[0];

	if (mode === "threshold") {
		const rules = colorConfig
			.slice(1)
			.map(parseThresholdRule)
			.filter((rule): rule is ThresholdRule => rule !== null);

		return data.map((item) => {
			const value = Number(item?.y);
			if (!Number.isFinite(value)) {
				return FALLBACK_COLOR;
			}
			const matchedRule = rules.find(
				(rule) => value >= rule.min && value < rule.max
			);
			return matchedRule?.color ?? FALLBACK_COLOR;
		});
	}

	if (mode === "label") {
		const labelMap = new Map<string, string>();
		colorConfig.slice(1).forEach((rule) => {
			const splitIndex = rule.lastIndexOf(":");
			if (splitIndex <= 0 || splitIndex === rule.length - 1) {
				return;
			}
			const label = rule.slice(0, splitIndex);
			const color = rule.slice(splitIndex + 1);
			labelMap.set(label, color);
		});

		return data.map((item) => {
			const key = String(item?.x ?? "");
			return labelMap.get(key) ?? FALLBACK_COLOR;
		});
	}

	// Sequential mode: keep original behavior and let ApexCharts cycle colors.
	return colorConfig;
}
