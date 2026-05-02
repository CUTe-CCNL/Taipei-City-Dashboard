<!-- Developed by Taipei Urban Intelligence Center 2023-2024-->

<script setup>
import { computed, ref, watch } from "vue";
import VueApexCharts from "vue3-apexcharts";
import { resolveChartColors } from "../utilities/chartColors";

/**
 * Props:
 * - chart_config: { unit?: string, color?: string[] }
 * - activeChart: current chart name controlled by parent component
 * - series: sunburst tree data, format:
 *   [{ name: "Root", children: [{ name: "A", value: 10 }, { name: "B", children: [...] }] }]
 * - map_config / map_filter / map_filter_on: map filtering contract aligned with TreemapChart/SankeyChart
 *
 * Strategy:
 * - Use multiple ApexCharts donut instances stacked concentrically to approximate a sunburst.
 * - This is not a true partition layout, so slight parent-child arc offset can occur.
 *
 * Minimal 2-layer config example:
 * {
 *   chart_config: { types: ["SunburstChart"], unit: "人", color: ["#4E79A7", "#F28E2B", "#E15759"] },
 *   chart_data: [{
 *     name: "全市",
 *     children: [
 *       { name: "信義區", value: 1200 },
 *       { name: "大安區", value: 1800 },
 *       { name: "中山區", value: 900 }
 *     ]
 *   }]
 * }
 *
 * Minimal 3-layer config example:
 * {
 *   chart_config: { types: ["SunburstChart"], unit: "人", color: ["#4E79A7", "#F28E2B", "#E15759"] },
 *   chart_data: [{
 *     name: "全市",
 *     children: [
 *       {
 *         name: "信義區",
 *         children: [
 *           { name: "信義里", value: 700 },
 *           { name: "三張里", value: 500 }
 *         ]
 *       },
 *       {
 *         name: "大安區",
 *         children: [
 *           { name: "敦化里", value: 1100 },
 *           { name: "臥龍里", value: 700 }
 *         ]
 *       }
 *     ]
 *   }]
 * }
 */

const CHART_NAME = "SunburstChart";
const MAX_DEPTH = 4;
const LIGHTNESS_STEP = -15;
const FALLBACK_PALETTE = [
	"#4E79A7",
	"#F28E2B",
	"#E15759",
	"#76B7B2",
	"#59A14F",
	"#EDC948",
	"#B07AA1",
	"#FF9DA7",
	"#9C755F",
	"#BAB0AC",
];

const props = defineProps([
	"chart_config",
	"activeChart",
	"series",
	"map_config",
	"map_filter",
	"map_filter_on",
]);

const emits = defineEmits([
	"filterByParam",
	"filterByLayer",
	"clearByParamFilter",
	"clearByLayerFilter",
	"fly",
]);

const selectedKey = ref(null);
const warnedDepth = ref(null);

const unitSuffix = computed(() => {
	if (!props.chart_config?.unit) {
		return "";
	}
	return ` ${escapeHtml(props.chart_config.unit)}`;
});

watch(
	() => props.activeChart,
	(value) => {
		if (value !== CHART_NAME) {
			selectedKey.value = null;
		}
	}
);

function escapeHtml(content) {
	return String(content ?? "")
		.replace(/&/g, "&amp;")
		.replace(/</g, "&lt;")
		.replace(/>/g, "&gt;")
		.replace(/"/g, "&quot;")
		.replace(/'/g, "&#39;");
}

function formatValue(value) {
	const parsedValue = Number(value);
	if (!Number.isFinite(parsedValue)) {
		return "0";
	}
	if (Math.abs(parsedValue) >= 1000) {
		return parsedValue.toLocaleString("zh-TW");
	}
	return (Math.round(parsedValue * 100) / 100).toString();
}

function toNonNegativeNumber(value) {
	const parsedValue = Number(value);
	if (!Number.isFinite(parsedValue) || parsedValue < 0) {
		return 0;
	}
	return parsedValue;
}

function normalizeColorHex(color) {
	if (typeof color !== "string") {
		return null;
	}
	const text = color.trim();
	if (!/^#([0-9A-Fa-f]{3}|[0-9A-Fa-f]{6})$/.test(text)) {
		return null;
	}
	if (text.length === 4) {
		return (
			"#" +
			text[1] +
			text[1] +
			text[2] +
			text[2] +
			text[3] +
			text[3]
		).toUpperCase();
	}
	return text.toUpperCase();
}

function hexToHsl(hexColor) {
	const normalizedHex = normalizeColorHex(hexColor);
	if (!normalizedHex) {
		return null;
	}

	const r = parseInt(normalizedHex.slice(1, 3), 16) / 255;
	const g = parseInt(normalizedHex.slice(3, 5), 16) / 255;
	const b = parseInt(normalizedHex.slice(5, 7), 16) / 255;

	const max = Math.max(r, g, b);
	const min = Math.min(r, g, b);
	const delta = max - min;

	let h = 0;
	const l = (max + min) / 2;
	let s = 0;

	if (delta !== 0) {
		s = delta / (1 - Math.abs(2 * l - 1));
		switch (max) {
		case r:
			h = ((g - b) / delta) % 6;
			break;
		case g:
			h = (b - r) / delta + 2;
			break;
		default:
			h = (r - g) / delta + 4;
		}
		h *= 60;
		if (h < 0) {
			h += 360;
		}
	}

	return {
		h,
		s: s * 100,
		l: l * 100,
	};
}

function hslToHex(h, s, l) {
	const hue = (((Number(h) % 360) + 360) % 360) / 360;
	const saturation = Math.min(100, Math.max(0, Number(s))) / 100;
	const lightness = Math.min(100, Math.max(0, Number(l))) / 100;

	if (!Number.isFinite(hue) || !Number.isFinite(saturation) || !Number.isFinite(lightness)) {
		return FALLBACK_PALETTE[0];
	}

	const chroma = (1 - Math.abs(2 * lightness - 1)) * saturation;
	const x = chroma * (1 - Math.abs(((hue * 6) % 2) - 1));
	const m = lightness - chroma / 2;

	let rPrime = 0;
	let gPrime = 0;
	let bPrime = 0;

	if (hue < 1 / 6) {
		rPrime = chroma;
		gPrime = x;
	} else if (hue < 2 / 6) {
		rPrime = x;
		gPrime = chroma;
	} else if (hue < 3 / 6) {
		gPrime = chroma;
		bPrime = x;
	} else if (hue < 4 / 6) {
		gPrime = x;
		bPrime = chroma;
	} else if (hue < 5 / 6) {
		rPrime = x;
		bPrime = chroma;
	} else {
		rPrime = chroma;
		bPrime = x;
	}

	const r = Math.round((rPrime + m) * 255);
	const g = Math.round((gPrime + m) * 255);
	const b = Math.round((bPrime + m) * 255);

	return `#${[r, g, b]
		.map((value) => Math.min(255, Math.max(0, value)).toString(16).padStart(2, "0"))
		.join("")
		.toUpperCase()}`;
}

function adjustLightness(hexColor, deltaPercent) {
	const hslColor = hexToHsl(hexColor);
	if (!hslColor) {
		return FALLBACK_PALETTE[0];
	}
	const adjustedLightness = Math.min(92, Math.max(12, hslColor.l + deltaPercent));
	return hslToHex(hslColor.h, hslColor.s, adjustedLightness);
}

function normalizeTreeNode(rawNode) {
	if (!rawNode || typeof rawNode !== "object") {
		return null;
	}

	const nameText = String(rawNode.name ?? "").trim();
	if (!nameText) {
		return null;
	}

	const rawChildren = Array.isArray(rawNode.children) ? rawNode.children : [];
	const children = rawChildren
		.map((childNode) => normalizeTreeNode(childNode))
		.filter((childNode) => childNode !== null);

	if (children.length > 0) {
		return {
			name: nameText,
			children,
			effectiveValue: children.reduce(
				(sum, childNode) => sum + childNode.effectiveValue,
				0
			),
		};
	}

	return {
		name: nameText,
		children: [],
		effectiveValue: toNonNegativeNumber(rawNode.value),
	};
}

function calculateTreeDepth(node) {
	if (!node || !Array.isArray(node.children) || node.children.length === 0) {
		return 1;
	}
	return (
		1 +
		Math.max(
			...node.children.map((childNode) => calculateTreeDepth(childNode))
		)
	);
}

function flattenByDepth(seriesRoot) {
	if (!seriesRoot || typeof seriesRoot !== "object") {
		return { rootName: "", rootValue: 0, levels: [] };
	}
	if (!String(seriesRoot.name ?? "").trim() || !Array.isArray(seriesRoot.children)) {
		return { rootName: "", rootValue: 0, levels: [] };
	}

	const normalizedRoot = normalizeTreeNode(seriesRoot);
	if (!normalizedRoot || normalizedRoot.children.length === 0) {
		return { rootName: "", rootValue: 0, levels: [] };
	}

	const totalDepth = calculateTreeDepth(normalizedRoot) - 1;
	if (totalDepth > MAX_DEPTH && warnedDepth.value !== totalDepth) {
		console.warn(
			`[SunburstChart] hierarchy depth ${totalDepth} exceeds ${MAX_DEPTH}. Rendering first ${MAX_DEPTH} levels only.`
		);
		warnedDepth.value = totalDepth;
	}
	if (totalDepth <= MAX_DEPTH) {
		warnedDepth.value = null;
	}

	const levelMap = new Map();
	const queue = normalizedRoot.children.map((childNode) => ({
		node: childNode,
		depth: 1,
		parentName: normalizedRoot.name,
		parentValue: normalizedRoot.effectiveValue,
		ancestorTopName: childNode.name,
	}));

	while (queue.length > 0) {
		const currentItem = queue.shift();
		if (!currentItem || currentItem.depth > MAX_DEPTH) {
			continue;
		}

		if (!levelMap.has(currentItem.depth)) {
			levelMap.set(currentItem.depth, []);
		}
		levelMap.get(currentItem.depth).push({
			name: currentItem.node.name,
			value: currentItem.node.effectiveValue,
			parentName: currentItem.parentName,
			parentValue: currentItem.parentValue,
			ancestorTopName: currentItem.ancestorTopName,
		});

		currentItem.node.children.forEach((childNode) => {
			queue.push({
				node: childNode,
				depth: currentItem.depth + 1,
				parentName: currentItem.node.name,
				parentValue: currentItem.node.effectiveValue,
				ancestorTopName: currentItem.ancestorTopName,
			});
		});
	}

	const levels = Array.from(levelMap.entries())
		.sort((a, b) => a[0] - b[0])
		.map(([depth, nodes]) => ({ depth, nodes }));

	return {
		rootName: normalizedRoot.name,
		rootValue: normalizedRoot.effectiveValue,
		levels,
	};
}

function assignColors(levels, chartColorConfig) {
	if (!Array.isArray(levels) || levels.length === 0) {
		return [];
	}

	const topLevelNodes = levels[0]?.nodes ?? [];
	if (topLevelNodes.length === 0) {
		return [];
	}

	const colorConfig =
		Array.isArray(chartColorConfig) && chartColorConfig.length > 0
			? chartColorConfig
			: FALLBACK_PALETTE;

	const topColors = resolveChartColors(
		colorConfig,
		topLevelNodes.map((node) => ({ x: node.name, y: node.value }))
	);

	const topColorMap = new Map();

	return levels.map((level) => ({
		...level,
		nodes: level.nodes.map((node, index) => {
			if (level.depth === 1) {
				const resolvedTopColor =
					normalizeColorHex(topColors[index]) ??
					FALLBACK_PALETTE[index % FALLBACK_PALETTE.length];
				topColorMap.set(node.name, resolvedTopColor);
				return {
					...node,
					color: resolvedTopColor,
				};
			}

			const baseColor =
				topColorMap.get(node.ancestorTopName) ??
				FALLBACK_PALETTE[index % FALLBACK_PALETTE.length];
			const color = adjustLightness(baseColor, LIGHTNESS_STEP * (level.depth - 1));

			return {
				...node,
				color,
			};
		}),
	}));
}

const rootNode = computed(() => {
	if (!Array.isArray(props.series) || props.series.length === 0) {
		return null;
	}
	const candidate = props.series[0];
	if (!candidate || typeof candidate !== "object") {
		return null;
	}
	if (!String(candidate.name ?? "").trim()) {
		return null;
	}
	if (!Array.isArray(candidate.children) || candidate.children.length === 0) {
		return null;
	}
	return candidate;
});

const flattenedChart = computed(() => flattenByDepth(rootNode.value));

const levels = computed(() =>
	assignColors(flattenedChart.value.levels, props.chart_config?.color)
);

const centerSummary = computed(() => ({
	name: flattenedChart.value.rootName,
	value: flattenedChart.value.rootValue,
}));

const renderedLevels = computed(() => {
	if (!Array.isArray(levels.value) || levels.value.length === 0) {
		return [];
	}

	const levelCount = levels.value.length;
	return levels.value.map((level, index) => {
		const reversedIndex = levelCount - index - 1;
		const inset = Math.max(0, reversedIndex * 10);
		const donutSize = Math.max(10, 70 - index * 18);

		return {
			depth: level.depth,
			nodes: level.nodes,
			series: level.nodes.map((node) => node.value),
			layerStyle: {
				inset: `${inset}%`,
			},
			chartOptions: {
				chart: {
					type: "donut",
					toolbar: {
						show: false,
					},
				},
				labels: level.nodes.map((node) => node.name),
				colors: level.nodes.map((node) => node.color),
				legend: {
					show: false,
				},
				dataLabels: {
					enabled: false,
				},
				stroke: {
					colors: ["#282a2c"],
					width: 2,
				},
				plotOptions: {
					pie: {
						expandOnClick: false,
						donut: {
							size: `${donutSize}%`,
						},
					},
				},
				tooltip: {
					custom: ({ dataPointIndex }) => {
						const targetNode = level.nodes[dataPointIndex];
						if (!targetNode) {
							return "";
						}
						const parentValue = Number(targetNode.parentValue);
						const percent =
							Number.isFinite(parentValue) && parentValue > 0
								? (targetNode.value / parentValue) * 100
								: 0;

						return (
							'<div class="chart-tooltip">' +
							`<h6>${escapeHtml(targetNode.name)}</h6>` +
							`<span>${formatValue(targetNode.value)}${unitSuffix.value} (${percent.toFixed(1)}%)</span>` +
							"</div>"
						);
					},
				},
			},
		};
	});
});

function handleDataSelection(level, config) {
	if (!props.map_filter || !props.map_filter_on) {
		return;
	}

	if (!level || !Array.isArray(level.nodes)) {
		return;
	}

	const dataPointIndex = Number(config?.dataPointIndex);
	if (!Number.isFinite(dataPointIndex) || dataPointIndex < 0) {
		return;
	}

	const targetNode = level.nodes[dataPointIndex];
	if (!targetNode || !targetNode.name) {
		return;
	}

	const selectedNodeKey = `${level.depth}-${targetNode.name}`;
	if (selectedKey.value === selectedNodeKey) {
		if (props.map_filter.mode === "byParam") {
			emits("clearByParamFilter", props.map_config);
		} else if (props.map_filter.mode === "byLayer") {
			emits("clearByLayerFilter", props.map_config);
		}
		selectedKey.value = null;
		return;
	}

	if (props.map_filter.mode === "byParam") {
		emits(
			"filterByParam",
			props.map_filter,
			props.map_config,
			targetNode.name,
			null
		);
		selectedKey.value = selectedNodeKey;
		return;
	}

	if (props.map_filter.mode === "byLayer") {
		emits("filterByLayer", props.map_config, targetNode.name);
		selectedKey.value = selectedNodeKey;
	}
}
</script>

<template>
  <div
    v-if="activeChart === CHART_NAME && renderedLevels.length > 0"
    class="sunburstchart"
  >
    <div class="sunburstchart-stack">
      <div
        v-for="level in renderedLevels"
        :key="`sunburst-level-${level.depth}`"
        class="sunburstchart-layer"
        :style="level.layerStyle"
      >
        <VueApexCharts
          width="100%"
          height="100%"
          type="donut"
          :options="level.chartOptions"
          :series="level.series"
          @data-point-selection="(_e, _ctx, cfg) => handleDataSelection(level, cfg)"
        />
      </div>
      <div class="sunburstchart-center">
        <h5>{{ centerSummary.name }}</h5>
        <h6>
          {{ formatValue(centerSummary.value) }}
          <span v-if="chart_config?.unit">{{ chart_config.unit }}</span>
        </h6>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.sunburstchart {
	position: relative;
	width: 100%;
	height: 100%;
	min-height: 220px;
	aspect-ratio: 1 / 1;
	overflow: visible;

	&-stack {
		position: relative;
		width: 100%;
		height: 100%;
	}

	&-layer {
		position: absolute;
		pointer-events: auto;
	}

	&-center {
		position: absolute;
		left: 50%;
		top: 50%;
		transform: translate(-50%, -50%);
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		text-align: center;
		z-index: 5;
		pointer-events: none;
		max-width: 46%;

		h5,
		h6 {
			margin: 0;
			line-height: 1.25;
			color: var(--color-complement-text);
			text-wrap: balance;
		}

		h6 {
			font-size: var(--font-m);
			font-weight: 400;
		}
	}
}
</style>
