<!-- Developed by Taipei Urban Intelligence Center 2023-2024-->

<script setup>
import { ref, computed, watch } from "vue";
import VueApexCharts from "vue3-apexcharts";
import { useThemeStore } from "../../store/themeStore";
import { resolveChartColors } from "../utilities/chartColors";
import { getThemeColor } from "../utilities/themeColors";

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
	"fly"
]);

const themeStore = useThemeStore();
const chartRenderKey = ref(0);
const componentBackgroundColor = computed(() => {
	themeStore.theme;
	return getThemeColor("--color-component-background");
});
const normalTextColor = computed(() => {
	themeStore.theme;
	return getThemeColor("--color-normal-text");
});

const MAX_ITEMS = 20;

const displayedSeries = computed(() => {
	if (!props.series?.length) return [];
	const sourceData = [...(props.series[0]?.data ?? [])].sort(
		(a, b) => b.y - a.y
	);

	if (sourceData.length <= MAX_ITEMS) {
		return [{ ...props.series[0], data: sourceData }];
	}

	const topItems = sourceData.slice(0, MAX_ITEMS);
	const otherSum = sourceData
		.slice(MAX_ITEMS)
		.reduce((acc, item) => acc + item.y, 0);

	return [
		{
			...props.series[0],
			data: [...topItems, { x: "其他", y: otherSum }],
		},
	];
});

const chartOptions = computed(() => ({
	chart: {
		borderRadius: 5,
		foreColor: normalTextColor.value,
		toolbar: {
			show: false,
		},
	},
	colors: resolveChartColors(
		props.chart_config.color,
		displayedSeries.value[0]?.data ?? []
	),
	dataLabels: {
		formatter: function (val) {
			return val;
		},
		style: {
			colors: (displayedSeries.value[0]?.data ?? []).map(
				() => normalTextColor.value
			),
		},
	},
	grid: {
		show: false,
	},
	legend: {
		show: false,
	},
	plotOptions: {
		treemap: {
			distributed: true,
			shadeIntensity: 0,
		},
	},
	stroke: {
		colors: [componentBackgroundColor.value],
		show: true,
		width: 2,
	},
	tooltip: {
		custom: function ({
			series,
			seriesIndex,
			dataPointIndex,
			w,
		}) {
			// The class "chart-tooltip" could be edited in /assets/styles/chartStyles.css
			return (
				'<div class="chart-tooltip">' +
				"<h6>" +
				w.globals.categoryLabels[dataPointIndex] +
				"</h6>" +
				"<span>" +
				series[seriesIndex][dataPointIndex] +
				` ${props.chart_config.unit}` +
				"</span>" +
				"</div>"
			);
		},
	},
	xaxis: {
		axisBorder: {
			show: false,
		},
		axisTicks: {
			show: false,
		},
		labels: {
			show: false,
		},
		type: "category",
	},
}));

watch(
	() => themeStore.theme,
	() => {
		chartRenderKey.value += 1;
	},
	{ immediate: true }
);

const sum = computed(() => {
	let sum = 0;
	props.series[0].data.forEach(
		(item) => (sum += item.y)
	);
	return Math.round(sum * 100) / 100;
});

const selectedIndex = ref(null);

function handleDataSelection(_e, _chartContext, config) {
	if (!props.map_filter || !props.map_filter_on) {
		return;
	}
	const categoryLabel = config.w.globals.categoryLabels[config.dataPointIndex];
	if (categoryLabel === "其他") {
		return;
	}
	if (
		`${config.dataPointIndex}-${config.seriesIndex}` !== selectedIndex.value
	) {
		// Supports filtering by xAxis
		if (props.map_filter.mode === "byParam") {
			emits(
				"filterByParam",
				props.map_filter,
				props.map_config,
				categoryLabel,
				null
			);
		}
		// Supports filtering by xAxis
		else if (props.map_filter.mode === "byLayer") {
			emits(
				"filterByLayer",
				props.map_config,
				categoryLabel
			);
		}
		selectedIndex.value = `${config.dataPointIndex}-${config.seriesIndex}`;
	} else {
		if (props.map_filter.mode === "byParam") {
			emits("clearByParamFilter", props.map_config);
		} else if (props.map_filter.mode === "byLayer") {
			emits("clearByLayerFilter", props.map_config);
		}
		selectedIndex.value = null;
	}
}
</script>

<template>
  <div
    v-if="activeChart === 'TreemapChart'"
    class="treemapchart"
  >
    <div class="treemapchart-title">
      <h5>總合</h5>
      <h6>{{ sum }} {{ chart_config.unit }}</h6>
    </div>
    <VueApexCharts
      :key="`treemap-${chartRenderKey}`"
      width="100%"
      type="treemap"
      :options="chartOptions"
      :series="displayedSeries"
      @data-point-selection="handleDataSelection"
    />
  </div>
</template>

<style scoped lang="scss">
.treemapchart {
	&-title {
		display: flex;
		justify-content: center;
		flex-direction: column;
		margin: 0.5rem 0 -0.5rem;

		h5 {
			margin: 0;
			color: var(--color-complement-text);
		}

		h6 {
			margin: 0;
			color: var(--color-complement-text);
			font-size: var(--font-m);
			font-weight: 400;
		}
	}
}
</style>
