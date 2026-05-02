<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import ApexSankey from "apexsankey";
import { resolveChartColors } from "../utilities/chartColors";

const CHART_NAME = "SankeyChart";

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

const chartContainer = ref(null);
const sankeyInstance = ref(null);
const resizeObserver = ref(null);
const selectedTitle = ref(null);
const renderFrameId = ref(null);
const tooltipId = `sankey-tooltip-${Math.random().toString(36).slice(2, 10)}`;

function getUnitSuffix() {
	if (!props.chart_config?.unit) {
		return "";
	}
	return ` ${escapeHtml(props.chart_config.unit)}`;
}

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
	return parsedValue.toLocaleString("zh-TW");
}

function getNodeTotals(nodes, edges) {
	const totalsMap = new Map();
	const inMap = new Map();
	const outMap = new Map();

	nodes.forEach((node) => {
		inMap.set(node.id, 0);
		outMap.set(node.id, 0);
	});

	edges.forEach((edge) => {
		const edgeValue = Number(edge.value);
		if (!Number.isFinite(edgeValue)) {
			return;
		}

		if (outMap.has(edge.source)) {
			outMap.set(edge.source, (outMap.get(edge.source) ?? 0) + edgeValue);
		}
		if (inMap.has(edge.target)) {
			inMap.set(edge.target, (inMap.get(edge.target) ?? 0) + edgeValue);
		}
	});

	nodes.forEach((node) => {
		totalsMap.set(
			node.id,
			Math.max(inMap.get(node.id) ?? 0, outMap.get(node.id) ?? 0)
		);
	});

	return totalsMap;
}

function applyNodeColors(nodes, edges) {
	const colorConfig = props.chart_config?.color;
	if (!Array.isArray(colorConfig) || colorConfig.length === 0) {
		return nodes;
	}

	const nodeTotals = getNodeTotals(nodes, edges);
	const colorInputs = nodes.map((node) => ({
		x: node.title,
		y: nodeTotals.get(node.id) ?? 0,
	}));
	const colors = resolveChartColors(colorConfig, colorInputs);
	if (!Array.isArray(colors) || colors.length === 0) {
		return nodes;
	}

	return nodes.map((node, index) => {
		const color = colors[index % colors.length];
		if (typeof color !== "string" || color.length === 0) {
			return node;
		}
		return { ...node, color };
	});
}

function parseRawNodesEdgesSeries(sankeySeries) {
	const nodes = sankeySeries.nodes
		.map((node) => {
			const id = node?.id === undefined || node?.id === null ? "" : String(node.id);
			const title =
				node?.title === undefined || node?.title === null
					? id
					: String(node.title);
			return { id, title };
		})
		.filter((node) => node.id.length > 0);

	const edges = sankeySeries.edges
		.map((edge) => {
			const source =
				edge?.source === undefined || edge?.source === null
					? ""
					: String(edge.source);
			const target =
				edge?.target === undefined || edge?.target === null
					? ""
					: String(edge.target);
			const value = Number(edge?.value);
			if (!source || !target || !Number.isFinite(value)) {
				return null;
			}

			const parsedEdge = {
				source,
				target,
				value,
			};
			if (edge?.type !== undefined && edge?.type !== null) {
				parsedEdge.type = String(edge.type);
			}
			return parsedEdge;
		})
		.filter((edge) => edge !== null);

	if (nodes.length === 0) {
		return null;
	}

	return { nodes, edges };
}

function parseLegacySeries(sankeySeries) {
	if (!Array.isArray(sankeySeries?.data)) {
		return null;
	}

	const targetId = "__sankey_total__";
	const targetTitle = props.chart_config?.sankey_target_title || "總計";
	const totalsMap = new Map();

	sankeySeries.data.forEach((item) => {
		const sourceTitle =
			item?.x === undefined || item?.x === null ? "" : String(item.x);
		const value = Number(item?.y);
		if (!sourceTitle || !Number.isFinite(value)) {
			return;
		}
		totalsMap.set(sourceTitle, (totalsMap.get(sourceTitle) ?? 0) + value);
	});

	if (totalsMap.size === 0) {
		return null;
	}

	const nodes = Array.from(totalsMap.keys()).map((sourceTitle) => ({
		id: sourceTitle,
		title: sourceTitle,
	}));
	nodes.push({ id: targetId, title: targetTitle });

	const edges = Array.from(totalsMap.entries()).map(([sourceTitle, value]) => ({
		source: sourceTitle,
		target: targetId,
		value,
	}));

	return { nodes, edges };
}

function parsePercentSeriesToSankey() {
	if (!Array.isArray(props.series) || props.series.length === 0) {
		return null;
	}
	if (!props.series.every((entry) => Array.isArray(entry?.data))) {
		return null;
	}

	const categories = Array.isArray(props.chart_config?.categories)
		? props.chart_config.categories
		: [];
	if (categories.length === 0) {
		return null;
	}

	const sourceNodes = props.series
		.map((entry, index) => {
			const title =
				entry?.name === undefined || entry?.name === null
					? `系列 ${index + 1}`
					: String(entry.name);
			const id = `series:${index}:${title}`;
			return { id, title, index };
		});

	const targetNodes = categories
		.map((category, index) => {
			const title =
				category === undefined || category === null
					? `分類 ${index + 1}`
					: String(category);
			const id = `category:${index}:${title}`;
			return { id, title, index };
		});

	const edges = [];
	sourceNodes.forEach((sourceNode) => {
		targetNodes.forEach((targetNode) => {
			const value = Number(
				props.series?.[sourceNode.index]?.data?.[targetNode.index]
			);
			if (!Number.isFinite(value) || value <= 0) {
				return;
			}
			edges.push({
				source: sourceNode.id,
				target: targetNode.id,
				value,
			});
		});
	});

	if (edges.length === 0) {
		return null;
	}

	return {
		nodes: [
			...sourceNodes.map(({ id, title }) => ({ id, title })),
			...targetNodes.map(({ id, title }) => ({ id, title })),
		],
		edges,
	};
}

function buildGraphData() {
	const sankeySeries = props.series?.[0];
	if (!sankeySeries && !Array.isArray(props.series)) {
		return null;
	}

	let parsed = null;
	if (
		sankeySeries &&
		Array.isArray(sankeySeries.nodes) &&
		Array.isArray(sankeySeries.edges)
	) {
		parsed = parseRawNodesEdgesSeries(sankeySeries);
	} else if (sankeySeries) {
		parsed = parseLegacySeries(sankeySeries);
	}
	if (!parsed) {
		parsed = parsePercentSeriesToSankey();
	}

	if (!parsed) {
		return null;
	}

	const { nodes, edges } = parsed;
	return {
		nodes: applyNodeColors(nodes, edges),
		edges,
	};
}

function clearChart() {
	if (renderFrameId.value !== null) {
		cancelAnimationFrame(renderFrameId.value);
		renderFrameId.value = null;
	}

	if (
		sankeyInstance.value &&
		typeof sankeyInstance.value.destroy === "function"
	) {
		sankeyInstance.value.destroy();
	}
	sankeyInstance.value = null;

	if (chartContainer.value) {
		chartContainer.value.innerHTML = "";
	}

	const tooltipElement = document.getElementById(tooltipId);
	if (tooltipElement) {
		tooltipElement.remove();
	}

}

function handleNodeClick(node) {
	if (!props.map_filter || !props.map_filter_on) {
		return;
	}

	const nodeTitle = node?.data?.title;
	if (!nodeTitle) {
		return;
	}

	if (selectedTitle.value === nodeTitle) {
		if (props.map_filter.mode === "byParam") {
			emits("clearByParamFilter", props.map_config);
		} else if (props.map_filter.mode === "byLayer") {
			emits("clearByLayerFilter", props.map_config);
		}
		selectedTitle.value = null;
		return;
	}

	if (props.map_filter.mode === "byParam") {
		emits(
			"filterByParam",
			props.map_filter,
			props.map_config,
			nodeTitle,
			null
		);
	} else if (props.map_filter.mode === "byLayer") {
		emits("filterByLayer", props.map_config, nodeTitle);
	}

	selectedTitle.value = nodeTitle;
}

function renderChart() {
	if (props.activeChart !== CHART_NAME || !chartContainer.value) {
		return;
	}

	const graphData = buildGraphData();
	if (!graphData) {
		clearChart();
		return;
	}

	const container = chartContainer.value;
	const width = container.clientWidth || 800;
	const height = container.clientHeight || Math.round(width / 1.6);

	clearChart();
	sankeyInstance.value = new ApexSankey(container, {
		width,
		height,
		enableToolbar: false,
		enableTooltip: true,
		tooltipId,
		tooltipTheme: "dark",
		canvasStyle: "border: none; box-sizing: border-box;",
		fontColor: "var(--color-normal-text)",
		fontWeight: "600",
		fontSize: "16px",
		onNodeClick: handleNodeClick,
		tooltipTemplate: ({ source, target, value }) => {
			return (
				`<div class="chart-tooltip">` +
				`<h6>${escapeHtml(source?.title ?? "-")} → ${escapeHtml(
					target?.title ?? "-"
				)}</h6>` +
				`<span>${formatValue(value)}${getUnitSuffix()}</span>` +
				`</div>`
			);
		},
		nodeTooltipTemplate: ({ node, value }) => {
			return (
				`<div class="chart-tooltip">` +
				`<h6>${escapeHtml(node?.title ?? "-")}</h6>` +
				`<span>${formatValue(value)}${getUnitSuffix()}</span>` +
				`</div>`
			);
		},
	});
	sankeyInstance.value.render(graphData);
}

function requestRender() {
	if (renderFrameId.value !== null) {
		cancelAnimationFrame(renderFrameId.value);
	}
	renderFrameId.value = requestAnimationFrame(() => {
		renderFrameId.value = null;
		renderChart();
	});
}

function handleResize() {
	if (props.activeChart !== CHART_NAME) {
		return;
	}
	requestRender();
}

function bindResizeObserver() {
	if (!chartContainer.value) {
		return;
	}
	unbindResizeObserver();

	if (typeof ResizeObserver !== "undefined") {
		resizeObserver.value = new ResizeObserver(() => {
			handleResize();
		});
		resizeObserver.value.observe(chartContainer.value);
		return;
	}

	window.addEventListener("resize", handleResize);
}

function unbindResizeObserver() {
	if (resizeObserver.value) {
		resizeObserver.value.disconnect();
		resizeObserver.value = null;
	}
	window.removeEventListener("resize", handleResize);
}

watch(
	() => props.activeChart,
	(newChart) => {
		if (newChart === CHART_NAME) {
			nextTick(() => {
				bindResizeObserver();
				requestRender();
			});
			return;
		}

		selectedTitle.value = null;
		unbindResizeObserver();
		clearChart();
	}
);

watch(
	() => [props.series, props.chart_config],
	() => {
		if (props.activeChart !== CHART_NAME) {
			return;
		}
		selectedTitle.value = null;
		requestRender();
	},
	{ deep: true }
);

onMounted(() => {
	if (props.activeChart !== CHART_NAME) {
		return;
	}
	bindResizeObserver();
	requestRender();
});

onBeforeUnmount(() => {
	unbindResizeObserver();
	clearChart();
	selectedTitle.value = null;
});
</script>

<template>
  <div
    v-if="activeChart === 'SankeyChart'"
    class="sankeychart"
  >
    <div
      ref="chartContainer"
      class="sankeychart-canvas"
    />
  </div>
</template>

<style scoped lang="scss">
.sankeychart {
	height: 100%;
	min-height: 220px;
	display: flex;
	align-items: stretch;
	position: relative;

	&-canvas {
		height: 100%;
		width: 100%;
		min-height: 220px;
		border-radius: 6px;
		background: transparent;
		color: var(--color-complement-text);
	}

	&-empty {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--color-complement-text);
		font-size: var(--font-s);
		pointer-events: none;
	}

	:deep(svg text) {
		fill: var(--color-normal-text) !important;
		font-weight: 600 !important;
		font-size: 16px !important;
		paint-order: stroke;
		stroke: rgba(0, 0, 0, 0.35);
		stroke-width: 0.8px;
		stroke-linejoin: round;
	}
}
</style>
