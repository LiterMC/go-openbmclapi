<script setup lang="ts">
import Button from 'primevue/button'
import ProgressSpinner from 'primevue/progressspinner'
import type { FileInfo } from '@/api/v0'

defineProps<{
	info: FileInfo
	content: string | null
}>()
</script>
<template>
	<div>
		<slot :info="info" :content="content" />
		<pre v-if="content" class="code-box"><code>{{ content }}</code></pre>
		<div v-else class="loading-box">
			<div class="flex-row-center">
				<ProgressSpinner
					style="width: 2em; height: 2em; margin: 0 0.5em 0 0"
					strokeWidth="5"
					fill="var(--surface-ground)"
					animationDuration="1s"
				/>
				<i>Loading...</i>
			</div>
		</div>
	</div>
</template>
<style scoped>
.code-box {
	width: 100%;
	height: 62vh;
	padding: 0.4rem;
	border-radius: 0.4rem;
	background: var(--surface-c);
	overflow: auto;
}

.loading-box {
	height: 62vh;
	padding: 1rem;
}
</style>
