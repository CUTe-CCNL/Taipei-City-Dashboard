<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useThemeStore } from "../../store/themeStore";
import { resolveChartColors } from "../utilities/chartColors";
import { getThemeColor } from "../utilities/themeColors";

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

const themeStore = useThemeStore();
const chartContainer = ref(null);
const resizeObserver = ref(null);
const selectedTitle = ref(null);
const renderFrameId = ref(null);
const svgWidth = ref(0);
const svgHeight = ref(0);
const renderedNodes = ref([]);
const renderedLinks = ref([]);
const tooltipElement = ref(null);
const gradientPrefix = `sankey-link-gradient-${Math.random()
	.toString(36)
	.slice(2, 10)}`;

const tooltipState = ref({
	visible: false,
	left: 0,
	top: 0,
	title: "",
	value: "",
});

const MIN_CHART_WIDTH = 320;
const MIN_CHART_HEIGHT = 220;
const NODE_PADDING = 12;
const MIN_NODE_HEIGHT = 8;
const NODE_CORNER_RADIUS = 6;

function getUnitSuffix() {
	if (!props.chart_config?.unit) {
		return "";
	}
	return ` ${String(props.chart_config.unit)}`;
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

function parseColor(rawColor) {
	const color = String(rawColor ?? "").trim().toLowerCase();
	if (!color) {
		return null;
	}

	const shortHexMatch = color.match(/^#([0-9a-f]{3})$/i);
	if (shortHexMatch) {
		const [r, g, b] = shortHexMatch[1].split("");
		return {
			r: parseInt(r + r, 16),
			g: parseInt(g + g, 16),
			b: parseInt(b + b, 16),
		};
	}

	const longHexMatch = color.match(/^#([0-9a-f]{6})$/i);
	if (longHexMatch) {
		return {
			r: parseInt(longHexMatch[1].slice(0, 2), 16),
			g: parseInt(longHexMatch[1].slice(2, 4), 16),
			b: parseInt(longHexMatch[1].slice(4, 6), 16),
		};
	}

	const rgbMatch = color.match(
		/^rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})/i
	);
	if (!rgbMatch) {
		return null;
	}

	return {
		r: Math.min(255, Number(rgbMatch[1])),
		g: Math.min(255, Number(rgbMatch[2])),
		b: Math.min(255, Number(rgbMatch[3])),
	};
}

function withAlpha(rawColor, alpha, fallbackColor = "#5a9cf8") {
	const color = parseColor(rawColor) || parseColor(fallbackColor);
	if (!color) {
		return `rgba(90, 156, 248, ${alpha})`;
	}
	return `rgba(${color.r}, ${color.g}, ${color.b}, ${alpha})`;
}

function createBandPath(x0, y0, x1, y1, thickness) {
	const curveOffset = Math.max(Math.abs(x1 - x0) * 0.45, 12);
	return [
		`M ${x0} ${y0}`,
		`C ${x0 + curveOffset} ${y0}, ${x1 - curveOffset} ${y1}, ${x1} ${y1}`,
		`L ${x1} ${y1 + thickness}`,
		`C ${x1 - curveOffset} ${y1 + thickness}, ${x0 + curveOffset} ${
			y0 + thickness
		}, ${x0} ${y0 + thickness}`,
		"Z",
	].join(" ");
}

function buildSankeyLayout(graphData, width, height) {
	const baseNodeColor = getThemeColor("--color-highlight") || "#5a9cf8";
	const fallbackLinkColor = getThemeColor("--color-complement-text") || "#888787";

	const nodes = graphData.nodes.map((node, index) => ({
		id: node.id,
		title: node.title,
		color: node.color || baseNodeColor,
		index,
		incoming: [],
		outgoing: [],
		inValue: 0,
		outValue: 0,
		value: 0,
		column: 0,
		x: 0,
		y: 0,
		height: 0,
		scale: 0,
		labelX: 0,
		labelY: 0,
		labelAnchor: "start",
	}));

	const nodeMap = new Map(nodes.map((node) => [node.id, node]));
	const links = [];

	graphData.edges.forEach((edge, index) => {
		const sourceNode = nodeMap.get(edge.source);
		const targetNode = nodeMap.get(edge.target);
		const value = Number(edge.value);
		if (!sourceNode || !targetNode || !Number.isFinite(value) || value <= 0) {
			return;
		}

		const link = {
			id: `${sourceNode.id}->${targetNode.id}:${index}`,
			source: sourceNode,
			target: targetNode,
			value,
		};
		sourceNode.outgoing.push(link);
		targetNode.incoming.push(link);
		sourceNode.outValue += value;
		targetNode.inValue += value;
		links.push(link);
	});

	nodes.forEach((node) => {
		node.value = Math.max(node.inValue, node.outValue, links.length === 0 ? 1 : 0);
	});

	const indegreeMap = new Map(nodes.map((node) => [node.id, node.incoming.length]));
	const queue = nodes.filter((node) => (indegreeMap.get(node.id) ?? 0) === 0);

	while (queue.length > 0) {
		const node = queue.shift();
		if (!node) {
			break;
		}
		node.outgoing.forEach((link) => {
			const nextColumn = node.column + 1;
			if (nextColumn > link.target.column) {
				link.target.column = nextColumn;
			}
			const nextIndegree = (indegreeMap.get(link.target.id) ?? 0) - 1;
			indegreeMap.set(link.target.id, nextIndegree);
			if (nextIndegree === 0) {
				queue.push(link.target);
			}
		});
	}

	const maxColumnCap = Math.max(nodes.length - 1, 0);
	for (let step = 0; step < nodes.length; step++) {
		let changed = false;
		links.forEach((link) => {
			const nextColumn = Math.min(maxColumnCap, link.source.column + 1);
			if (nextColumn > link.target.column) {
				link.target.column = nextColumn;
				changed = true;
			}
		});
		if (!changed) {
			break;
		}
	}

	const maxColumn = nodes.reduce(
		(currentMax, node) => Math.max(currentMax, node.column),
		0
	);
	const columns = Array.from({ length: maxColumn + 1 }, () => []);
	nodes.forEach((node) => {
		columns[node.column].push(node);
	});
	columns.forEach((columnNodes) => {
		columnNodes.sort((a, b) => b.value - a.value || a.index - b.index);
	});

	const chartWidth = Math.max(MIN_CHART_WIDTH, width);
	const chartHeight = Math.max(MIN_CHART_HEIGHT, height);
	const margin = { top: 16, right: 20, bottom: 16, left: 20 };
	const nodeWidth = Math.max(14, Math.min(26, chartWidth * 0.035));
	const innerWidth = Math.max(1, chartWidth - margin.left - margin.right - nodeWidth);
	const innerHeight = Math.max(1, chartHeight - margin.top - margin.bottom);
	const stepX = columns.length > 1 ? innerWidth / (columns.length - 1) : 0;

	let flowScale = Number.POSITIVE_INFINITY;
	columns.forEach((columnNodes) => {
		if (columnNodes.length === 0) {
			return;
		}
		const totalValue = columnNodes.reduce((sum, node) => sum + node.value, 0);
		if (totalValue <= 0) {
			return;
		}
		const columnGaps = NODE_PADDING * Math.max(columnNodes.length - 1, 0);
		const nextScale = (innerHeight - columnGaps) / totalValue;
		flowScale = Math.min(flowScale, nextScale);
	});
	if (!Number.isFinite(flowScale) || flowScale <= 0) {
		flowScale = innerHeight / Math.max(nodes.length, 1);
	}

	columns.forEach((columnNodes, columnIndex) => {
		const x = margin.left + stepX * columnIndex;
		const provisionalHeights = columnNodes.map((node) =>
			Math.max(MIN_NODE_HEIGHT, node.value * flowScale)
		);
		const rawHeightSum = provisionalHeights.reduce((sum, h) => sum + h, 0);
		const gapSum = NODE_PADDING * Math.max(columnNodes.length - 1, 0);
		const renderedHeight = rawHeightSum + gapSum;
		const overflowScale =
			renderedHeight > innerHeight && renderedHeight > 0
				? innerHeight / renderedHeight
				: 1;
		const heights = provisionalHeights.map((h) => h * overflowScale);
		const totalHeight =
			heights.reduce((sum, h) => sum + h, 0) +
			NODE_PADDING * Math.max(columnNodes.length - 1, 0);

		let currentY = margin.top + Math.max((innerHeight - totalHeight) / 2, 0);
		columnNodes.forEach((node, index) => {
			node.x = x;
			node.y = currentY;
			node.height = heights[index];
			node.scale = node.value > 0 ? node.height / node.value : 0;
			currentY += node.height + NODE_PADDING;
		});
	});

	const chartMidColumn = maxColumn / 2;
	nodes.forEach((node) => {
		const anchor = node.column <= chartMidColumn ? "start" : "end";
		node.labelAnchor = anchor;
		node.labelX = anchor === "start" ? node.x + nodeWidth + 8 : node.x - 8;
		node.labelY = node.y + node.height / 2;
	});

	nodes.forEach((node) => {
		node.outgoing.sort(
			(a, b) =>
				a.target.y + a.target.height / 2 - (b.target.y + b.target.height / 2)
		);
		node.incoming.sort(
			(a, b) =>
				a.source.y + a.source.height / 2 - (b.source.y + b.source.height / 2)
		);
	});

	const sourceOffsets = new Map(nodes.map((node) => [node.id, 0]));
	const targetOffsets = new Map(nodes.map((node) => [node.id, 0]));

	const positionedLinks = links.map((link, linkIndex) => {
		const sourceScale = link.source.scale || flowScale;
		const targetScale = link.target.scale || flowScale;
		const thickness = Math.max(1, link.value * Math.min(sourceScale, targetScale));
		const sourceOffset = sourceOffsets.get(link.source.id) ?? 0;
		const targetOffset = targetOffsets.get(link.target.id) ?? 0;
		const y0 = link.source.y + sourceOffset;
		const y1 = link.target.y + targetOffset;

		sourceOffsets.set(link.source.id, sourceOffset + thickness);
		targetOffsets.set(link.target.id, targetOffset + thickness);

		const x0 = link.source.x + nodeWidth;
		const x1 = link.target.x;
		const sourceColor = link.source.color || fallbackLinkColor;
		const targetColor = link.target.color || fallbackLinkColor;
		const startColor = withAlpha(sourceColor, 0.45, baseNodeColor);
		const endColor = withAlpha(targetColor, 0.26, fallbackLinkColor);
		const gradientId = `${gradientPrefix}-${linkIndex}-${link.id.replace(
			/[^a-z0-9_-]/gi,
			"_"
		)}`;
		return {
			id: link.id,
			sourceTitle: link.source.title,
			targetTitle: link.target.title,
			value: link.value,
			fill: `url(#${gradientId})`,
			gradientId,
			gradientX1: x0,
			gradientY1: y0,
			gradientX2: x1,
			gradientY2: y1,
			gradientStartColor: startColor,
			gradientEndColor: endColor,
			path: createBandPath(x0, y0, x1, y1, thickness),
		};
	});

	return {
		width: chartWidth,
		height: chartHeight,
		nodes: nodes.map((node) => ({
			id: node.id,
			title: node.title,
			value: node.value,
			color: node.color,
			x: node.x,
			y: node.y,
			width: nodeWidth,
			height: node.height,
			labelX: node.labelX,
			labelY: node.labelY,
			labelAnchor: node.labelAnchor,
		})),
		links: positionedLinks,
	};
}

function hideTooltip() {
	tooltipState.value.visible = false;
}

function setTooltipPosition(event) {
	if (!chartContainer.value) {
		return;
	}

	const rect = chartContainer.value.getBoundingClientRect();
	const padding = 8;
	const offset = 12;
	const pointerX = event.clientX - rect.left;
	const pointerY = event.clientY - rect.top;
	const tooltipWidth = tooltipElement.value?.offsetWidth ?? 0;
	const tooltipHeight = tooltipElement.value?.offsetHeight ?? 0;

	let left = pointerX + offset;
	let top = pointerY + offset;

	if (left + tooltipWidth > chartContainer.value.clientWidth - padding) {
		left = pointerX - tooltipWidth - offset;
	}
	if (top + tooltipHeight > chartContainer.value.clientHeight - padding) {
		top = pointerY - tooltipHeight - offset;
	}

	const maxLeft = Math.max(
		padding,
		chartContainer.value.clientWidth - tooltipWidth - padding
	);
	const maxTop = Math.max(
		padding,
		chartContainer.value.clientHeight - tooltipHeight - padding
	);

	tooltipState.value.left = Math.max(padding, Math.min(maxLeft, left));
	tooltipState.value.top = Math.max(padding, Math.min(maxTop, top));
}

function updateTooltipPosition(event) {
	if (!tooltipState.value.visible) {
		return;
	}
	setTooltipPosition(event);
}

function showNodeTooltip(event, node) {
	tooltipState.value.title = node?.title || "-";
	tooltipState.value.value = `${formatValue(node?.value)}${getUnitSuffix()}`;
	tooltipState.value.visible = true;
	nextTick(() => {
		setTooltipPosition(event);
	});
}

function showLinkTooltip(event, link) {
	tooltipState.value.title = `${link?.sourceTitle || "-"} → ${
		link?.targetTitle || "-"
	}`;
	tooltipState.value.value = `${formatValue(link?.value)}${getUnitSuffix()}`;
	tooltipState.value.visible = true;
	nextTick(() => {
		setTooltipPosition(event);
	});
}

function isNodeClickable() {
	return Boolean(props.map_filter && props.map_filter_on);
}

function getNodeFill(node) {
	if (!selectedTitle.value) {
		return node.color;
	}
	if (selectedTitle.value === node.title) {
		return node.color;
	}
	return withAlpha(node.color, 0.35);
}

function getNodeStroke(node) {
	if (selectedTitle.value === node.title) {
		return getThemeColor("--color-highlight") || node.color;
	}
	return withAlpha(node.color, 0.9);
}

function getLinkOpacity(link) {
	if (!selectedTitle.value) {
		return 1;
	}
	return link.sourceTitle === selectedTitle.value ||
		link.targetTitle === selectedTitle.value
		? 1
		: 0.18;
}

function clearChart() {
	if (renderFrameId.value !== null) {
		cancelAnimationFrame(renderFrameId.value);
		renderFrameId.value = null;
	}

	renderedNodes.value = [];
	renderedLinks.value = [];
	svgWidth.value = 0;
	svgHeight.value = 0;
	hideTooltip();
}

function handleNodeClick(node) {
	if (!isNodeClickable()) {
		return;
	}

	const nodeTitle = node?.title;
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

	const layout = buildSankeyLayout(graphData, width, height);
	if (!layout) {
		clearChart();
		return;
	}

	svgWidth.value = layout.width;
	svgHeight.value = layout.height;
	renderedNodes.value = layout.nodes;
	renderedLinks.value = layout.links;
	hideTooltip();
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

watch(
	() => themeStore.theme,
	() => {
		if (props.activeChart !== CHART_NAME) {
			return;
		}
		requestRender();
	},
	{ immediate: true }
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
      @mouseleave="hideTooltip"
    >
      <svg
        v-if="renderedNodes.length > 0 && svgWidth > 0 && svgHeight > 0"
        class="sankeychart-svg"
        :viewBox="`0 0 ${svgWidth} ${svgHeight}`"
        preserveAspectRatio="xMidYMid meet"
        role="img"
        aria-label="Sankey flow chart"
      >
        <defs>
          <linearGradient
            v-for="link in renderedLinks"
            :id="link.gradientId"
            :key="link.gradientId"
            gradientUnits="userSpaceOnUse"
            :x1="link.gradientX1"
            :y1="link.gradientY1"
            :x2="link.gradientX2"
            :y2="link.gradientY2"
          >
            <stop
              offset="0%"
              :stop-color="link.gradientStartColor"
            />
            <stop
              offset="100%"
              :stop-color="link.gradientEndColor"
            />
          </linearGradient>
        </defs>
        <g class="sankeychart-links">
          <path
            v-for="link in renderedLinks"
            :key="link.id"
            class="sankeychart-link"
            :d="link.path"
            :fill="link.fill"
            :opacity="getLinkOpacity(link)"
            @mouseenter="showLinkTooltip($event, link)"
            @mousemove="updateTooltipPosition"
            @mouseleave="hideTooltip"
          />
        </g>
        <g class="sankeychart-nodes">
          <g
            v-for="node in renderedNodes"
            :key="node.id"
            class="sankeychart-node"
            :class="{
              'is-clickable': isNodeClickable(),
              'is-selected': selectedTitle === node.title,
            }"
            @click="handleNodeClick(node)"
            @mouseenter="showNodeTooltip($event, node)"
            @mousemove="updateTooltipPosition"
            @mouseleave="hideTooltip"
          >
            <rect
              :x="node.x"
              :y="node.y"
              :width="node.width"
              :height="node.height"
              :rx="NODE_CORNER_RADIUS"
              :fill="getNodeFill(node)"
              :stroke="getNodeStroke(node)"
              stroke-width="1.2"
            />
            <text
              :x="node.labelX"
              :y="node.labelY"
              :text-anchor="node.labelAnchor"
              dominant-baseline="middle"
            >
              {{ node.title }}
            </text>
          </g>
        </g>
      </svg>
      <div
        v-else
        class="sankeychart-empty"
      >
        暫無資料
      </div>
      <div
        v-if="tooltipState.visible"
        ref="tooltipElement"
        class="sankeychart-tooltip chart-tooltip"
        :style="{
          left: `${tooltipState.left}px`,
          top: `${tooltipState.top}px`,
        }"
      >
        <h6>{{ tooltipState.title }}</h6>
        <span>{{ tooltipState.value }}</span>
      </div>
    </div>
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
		position: relative;
		overflow: hidden;
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

	&-svg {
		width: 100%;
		height: 100%;
		display: block;
	}

	&-link {
		transition: opacity 0.15s ease;
	}

	&-node {
		transition: opacity 0.15s ease;

		&.is-clickable {
			cursor: pointer;
		}
	}

	&-tooltip {
		position: absolute;
		pointer-events: none;
		z-index: 12;
		max-width: 280px;
		white-space: nowrap;
	}

	:deep(text) {
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
