<script setup lang="ts">
import { watch, ref, reactive, nextTick, onMounted } from 'vue'
import Button from 'primevue/button'
import { tr } from '@/lang'

type Status = 'enabled' | 'disabled' | 'error'

const props = defineProps<{
	status: Status
}>()

const statusListRef = ref<HTMLDivElement>()
const statusList = reactive(['enabled', 'disabled', 'error'])

function waitForAnimationFrame(): Promise<number> {
	return new Promise((re) => requestAnimationFrame(re))
}

onMounted(() => {
	if (props.status === statusList[0]) {
		statusList.unshift(statusList.pop() as string)
	} else if (props.status === statusList[2]) {
		statusList.push(statusList.shift() as string)
	}
	watch(
		() => props.status,
		async (status) => {
			const statusListElem = statusListRef.value
			if (!statusListElem) {
				return
			}
			if (status === statusList[0]) {
				// move down
				statusList.unshift(statusList.pop() as string)
				statusListElem.animate(
					{
						top: ['-200%', '-100%'],
						easing: ['ease', 'ease'],
					},
					500,
				)
			} else if (status === statusList[2]) {
				// move up
				statusList.push(statusList.shift() as string)
				statusListElem.animate(
					{
						top: ['0%', '-100%'],
						easing: ['ease', 'ease'],
					},
					500,
				)
			} else {
				return
			}
		},
	)
})
</script>

<template>
	<Button class="info-status" :status="status">
		<div ref="statusListRef" class="status-list">
			<span v-for="s in statusList" :key="s">
				{{ tr(`badge.server.status.${s}`) }}
			</span>
		</div>
	</Button>
</template>
<style scoped>
.info-status {
	--flash-from: unset;
	--flash-out: var(--flash-from);
	display: inline-flex !important;
	flex-direction: row;
	align-items: center;
	position: relative;
	height: 2.7rem;
	margin: 0.5rem;
	padding: 0;
	border: none;
	border-radius: 0.2rem;
	font-weight: 800;
	overflow: hidden;
	user-select: none;
	cursor: pointer;
	transition: 1s background-color ease-out;
}

.info-status[status='enabled'] {
	--flash-from: #fff;
	--flash-to: #11dfc3;
	color: #fff;
	background-color: #28a745;
	animation: flash 1s infinite;
}

.info-status[status='disabled'] {
	--flash-from: #fff;
	--flash-to: #e61a05;
	color: #fff;
	background-color: #f89f1b;
	animation: flash 3s infinite;
}

.info-status[status='error'] {
	--flash-from: #8a8dac;
	color: #fff;
	background-color: #bfadad;
}

.info-status::before {
	content: ' ';
	display: inline-block;
	width: 1.05rem;
	height: 1.05rem;
	margin: 0.5rem;
	border: solid #fff 0.25rem;
	border-radius: 50%;
	background-color: var(--flash-out);
	box-shadow: #fff8 inset 0 0 2px;
	transition: background-color 0.15s;
}

.status-list {
	display: flex;
	flex-direction: column;
	align-items: baseline;
	height: 100%;
	margin-right: 0.7rem;
	overflow: visible;
	position: relative;
	top: -100%;
}

.status-list > span {
	display: flex;
	flex-direction: row;
	flex-shrink: 0;
	align-items: center;
	height: 100%;
}
</style>
