<script setup>
import { onMounted, onUnmounted, ref } from "vue";

const MAX_RETRIES = 5;
const RETRY_DELAY_MS = 1000;
const REFRESH_INTERVAL_MS = 60000;

const props = defineProps({
	camera: {
		type: Object,
		required: true,
	},
});

const cardRef = ref(null);
const imgRef = ref(null);
const isLoading = ref(false);
const isOffline = ref(false);
const retryCount = ref(0);
const isInViewport = ref(false);

let observer = null;
let retryTimer = null;
let refreshTimer = null;

const clearRetryTimer = () => {
	if (retryTimer) {
		clearTimeout(retryTimer);
		retryTimer = null;
	}
};

const clearRefreshTimer = () => {
	if (refreshTimer) {
		clearInterval(refreshTimer);
		refreshTimer = null;
	}
};

const getStreamUrl = () => {
	const separator = props.camera.url.includes("?") ? "&" : "?";
	return `${props.camera.url}${separator}_t=${Date.now()}`;
};

const startRefreshTimer = () => {
	clearRefreshTimer();
	refreshTimer = setInterval(() => {
		if (isInViewport.value && !isOffline.value) {
			startStream();
		}
	}, REFRESH_INTERVAL_MS);
};

const startStream = () => {
	if (!imgRef.value || !isInViewport.value || isOffline.value) {
		return;
	}
	clearRetryTimer();
	isLoading.value = true;
	imgRef.value.src = getStreamUrl();
	startRefreshTimer();
};

const stopStream = () => {
	clearRetryTimer();
	clearRefreshTimer();
	isLoading.value = false;
	if (imgRef.value) {
		imgRef.value.src = "";
	}
};

const handleLoad = () => {
	isLoading.value = false;
	retryCount.value = 0;
};

const handleError = () => {
	clearRetryTimer();
	clearRefreshTimer();
	if (!isInViewport.value) {
		return;
	}
	if (retryCount.value >= MAX_RETRIES) {
		isOffline.value = true;
		isLoading.value = false;
		return;
	}
	retryCount.value += 1;
	retryTimer = setTimeout(() => {
		startStream();
	}, RETRY_DELAY_MS);
};

const manualRetry = () => {
	retryCount.value = 0;
	isOffline.value = false;
	startStream();
};

onMounted(() => {
	observer = new IntersectionObserver(
		(entries) => {
			const [entry] = entries;
			isInViewport.value = entry.isIntersecting;
			if (entry.isIntersecting) {
				retryCount.value = 0;
				isOffline.value = false;
				startStream();
				return;
			}
			stopStream();
		},
		{ rootMargin: "100px" },
	);

	if (cardRef.value) {
		observer.observe(cardRef.value);
	}
});

onUnmounted(() => {
	if (observer) {
		observer.disconnect();
		observer = null;
	}
	stopStream();
});
</script>

<template>
  <article
    ref="cardRef"
    class="cctv-card"
  >
    <div class="cctv-card__stream">
      <img
        ref="imgRef"
        :alt="camera.name"
        @load="handleLoad"
        @error="handleError"
      >
      <div
        v-if="isLoading"
        class="cctv-card__loading"
      >
        載入中...
      </div>
      <div
        v-if="isOffline"
        class="cctv-card__offline"
      >
        <p>攝影機離線</p>
        <button
          type="button"
          @click="manualRetry"
        >
          重新連線
        </button>
      </div>
    </div>
    <p class="cctv-card__name">
      {{ camera.name }}
    </p>
  </article>
</template>

<style scoped lang="scss">
.cctv-card {
	border: 1px solid var(--color-border);
	border-radius: 8px;
	padding: 10px;
	background-color: var(--color-component-background);

	&__stream {
		position: relative;
		width: 100%;
		aspect-ratio: 16 / 9;
		border-radius: 6px;
		overflow: hidden;
		background-color: #111;
	}

	img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		display: block;
	}

	&__loading,
	&__offline {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		background-color: rgba(0, 0, 0, 0.6);
		color: #fff;
	}

	&__offline {
		flex-direction: column;
		gap: 8px;

		button {
			padding: 4px 10px;
			border: 1px solid #fff;
			border-radius: 6px;
			color: #fff;
			background: transparent;
			cursor: pointer;
		}
	}

	&__name {
		margin-top: 8px;
		font-size: var(--font-m);
	}
}
</style>
