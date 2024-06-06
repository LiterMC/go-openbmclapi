<script setup lang="ts">
import Button from 'primevue/button'
import type { FileInfo } from '@/api/v0'

defineProps<{
	info: FileInfo
	content: string
}>()
</script>
<template>
	<div>
		<slot :info="info" :content="content" />
		<pre class="code-box"><code>{{ content }}</code></pre>
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
</style>
