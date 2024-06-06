<script setup lang="ts">
import Card from 'primevue/card'
import type { FileInfo } from '@/api/v0'
import { formatNumber, formatBytes } from '@/utils'

defineProps<{
	name: string
	files: FileInfo[] | null
}>()

defineEmits<{
	(e: 'click', file: FileInfo, index: number): void
}>()
</script>
<template>
	<Card>
		<template #title>
			<div class="flex-row-center">
				<lable>{{ name }}</lable>
			</div>
		</template>
		<template #content>
			<template v-if="!files">
				<i>Loading...</i>
			</template>
			<template v-else>
				<div class="file-list-box">
					<div v-for="(file, i) in files" v-key="file.name" class="file-elem">
						<div class="file-name flex-row-center" @click="$emit('click', file, i)">
							{{ file.name }}
						</div>
						<div class="flex-row-center">
							<div class="file-size">{{ formatBytes(file.size) }}</div>
							<slot :index="i" :file="file" />
						</div>
					</div>
				</div>
			</template>
		</template>
	</Card>
</template>
<style scoped>
.file-list-box {
	display: flex;
	flex-direction: column;
	width: 100%;
}

.file-elem {
	display: flex;
	flex-direction: row;
	justify-content: space-between;
	width: 100%;
	padding: 0.5rem 0.2rem;
	border-bottom: var(--surface-border) 1px solid;
}

.file-name {
	width: 10rem;
	padding-right: 0.3rem;
	font-weight: bold;
	cursor: pointer;
	user-select: all;
}

.file-name:hover {
	color: var(--surface-900);
	text-decoration: underline;
}

.file-size {
	width: 5rem;
	margin-right: 0.3rem;
	font-weight: 200;
	user-select: none;
}
</style>
