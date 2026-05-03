<!-- Developed by Taipei Urban Intelligence Center 2023-2024-->

<script setup>
import { ref, computed, watch } from "vue";
import VueApexCharts from "vue3-apexcharts";
import { useThemeStore } from "../../store/themeStore";
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
const complementTextColor = computed(() => {
	themeStore.theme;
	return getThemeColor("--color-complement-text");
});

const chartOptions = computed(() => ({
	chart: {
		foreColor: complementTextColor.value,
		stacked: true,
		stackType: "100%",
		toolbar: {
			show: false,
		},
	},
	colors: props.chart_config.types.includes("GuageChart")
		? [props.chart_config.color[0], "#777"]
		: props.chart_config.color,
	dataLabels: {
		textAnchor: "start",
		style: {
			colors: props.series.map(() => normalTextColor.value),
		},
	},
	grid: {
		show: false,
	},
	legend: {
		offsetY: 20,
		position: "top",
		show: props.series.length > 2 ? true : false,
	},
	plotOptions: {
		bar: {
			borderRadius: 5,
			horizontal: true,
		},
	},
	stroke: {
		colors: [componentBackgroundColor.value],
		show: true,
		width: 2,
	},
	tooltip: {
		// The class "chart-tooltip" could be edited in /assets/styles/chartStyles.css
		custom: function ({
			series,
			seriesIndex,
			dataPointIndex,
			w,
		}) {
			return (
				'<div class="chart-tooltip">' +
				"<h6>" +
				w.globals.seriesNames[seriesIndex] +
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
		categories: props.chart_config.categories
			? props.chart_config.categories
			: [],
		labels: {
			show: false,
		},
		type: "category",
	},
}));

const chartHeight = computed(() => {
	return `${50 + props.series[0].data.length * 30}`;
});

watch(
	() => themeStore.theme,
	() => {
		chartRenderKey.value += 1;
	},
	{ immediate: true }
);

const selectedIndex = ref(null);

function handleDataSelection(_e, _chartContext, config) {
	if (!props.map_filter || !props.map_filter_on) {
		return;
	}
	if (
		`${config.dataPointIndex}-${config.seriesIndex}` !== selectedIndex.value
	) {
		// Supports filtering by xAxis + yAxis
		if (props.map_filter.mode === "byParam") {
			emits(
				"filterByParam",
				props.map_filter,
				props.map_config,
				config.w.globals.labels[config.dataPointIndex],
				config.w.globals.seriesNames[config.seriesIndex]
			);
		}
		// Supports filtering by xAxis
		else if (props.map_filter.mode === "byLayer") {
			emits(
				"filterByLayer",
				props.map_config,
				config.w.globals.labels[config.dataPointIndex]
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
    v-if="activeChart === 'BarPercentChart'"
  >
    <VueApexCharts
      :key="`bar-percent-${chartRenderKey}`"
      type="bar"
      width="100%"
      :height="chartHeight"
      :options="chartOptions"
      :series="series"
      @data-point-selection="handleDataSelection"
    />
  </div>
</template>
