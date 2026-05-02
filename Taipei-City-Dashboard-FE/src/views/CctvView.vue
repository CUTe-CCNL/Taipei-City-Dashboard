<script setup>
import { computed, ref } from "vue";
import cctvList from "../assets/configs/cctvList";
import CctvCard from "../components/cctv/CctvCard.vue";

const columns = ref(3);
const columnOptions = [2, 3, 4];

const gridStyle = computed(() => ({
	gridTemplateColumns: `repeat(${columns.value}, minmax(0, 1fr))`,
}));
</script>

<template>
  <main class="cctv-view">
    <section class="cctv-view__header">
      <h1>CCTV 即時監看</h1>
      <div class="cctv-view__controls">
        <button
          v-for="count in columnOptions"
          :key="count"
          type="button"
          :class="{ active: columns === count }"
          @click="columns = count"
        >
          {{ count }} 欄
        </button>
      </div>
    </section>

    <section
      class="cctv-view__grid"
      :style="gridStyle"
    >
      <CctvCard
        v-for="camera in cctvList"
        :key="camera.id"
        :camera="camera"
      />
    </section>
  </main>
</template>

<style scoped lang="scss">
.cctv-view {
	flex: 1;
	min-height: 0;
	padding: 20px;
	display: flex;
	flex-direction: column;
	gap: 16px;
	background-color: var(--color-background);
	overflow-y: auto;
	overflow-x: hidden;

	&__header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;

		h1 {
			font-size: var(--font-xl);
			font-weight: 500;
		}
	}

	&__controls {
		display: flex;
		gap: 8px;

		button {
			padding: 4px 10px;
			border: 1px solid var(--color-border);
			border-radius: 6px;
			transition: all 0.2s;

			&.active {
				border-color: var(--color-highlight);
				color: var(--color-highlight);
			}
		}
	}

	&__grid {
		display: grid;
		gap: 12px;
	}
}

@media screen and (max-width: 1024px) {
	.cctv-view {
		&__header {
			flex-direction: column;
			align-items: flex-start;
		}
	}
}
</style>
