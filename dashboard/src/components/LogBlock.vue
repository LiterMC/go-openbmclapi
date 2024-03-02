<script setup lang="ts">
import { ref, reactive, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'

interface Log {
	_inc?: number
	time: number
	lvl: string
	log: string
}

const box = ref<HTMLElement>()
const logs = reactive<Log[]>([])
const MAX_LOG_LENGTH = 1024 * 4

var logInc = 0
var focusLastLog = true
var justSplicedLog = false

function pushLog(log: Log): void {
	log._inc = logInc = (logInc + 1) % 65536
	logs.push(log)
	const minI = logs.length - MAX_LOG_LENGTH
	if (minI > 0) {
		logs.splice(0, minI)
		justSplicedLog = true
	}
}

var boxLastPosTop = 0

function onScrollBox(): void {
	if (document.hidden || !box.value) {
		return
	}
	const scrolledY = box.value.scrollTop - boxLastPosTop
	boxLastPosTop = box.value.scrollTop
	if (scrolledY < 0) {
		if (justSplicedLog) {
			justSplicedLog = false
		} else {
			focusLastLog = false
		}
	}
}

function onVisibilityChange(): void {
	if (document.hidden || !box.value || !focusLastLog) {
		return
	}
	box.value.scroll({
		top: box.value.scrollHeight,
		behavior: 'smooth',
	})
}

watch(logs, async (logs: Log[]) => {
	if (document.hidden || !box.value) {
		return
	}
	const diff = box.value.scrollHeight - (box.value.scrollTop + box.value.clientHeight)
	focusLastLog ||= diff < 5
	if (focusLastLog && !document.hidden) {
		await nextTick()
		box.value.scroll({
			top: box.value.scrollHeight,
			behavior: diff < box.value.clientHeight ? 'smooth' : 'auto',
		})
	}
})

function formatDate(date: Date): string {
	const pad2 = (n: number) => n.toString().padStart(2, '0')
	return `${pad2(date.getHours())}:${pad2(date.getMinutes())}:${pad2(date.getSeconds())}`
}

defineExpose({
	pushLog,
})

onMounted(() => {
	if (box.value) {
		boxLastPosTop = box.value.scrollTop
	}
	document.addEventListener('visibilitychange', onVisibilityChange)
})

onBeforeUnmount(() => {
	document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>
<template>
	<div class="outer">
		<div class="inner" ref="box" @scroll="onScrollBox">
			<div class="box">
				<div v-for="log in logs" :key="`${log.time},${log._inc}`" class="line" :level="log.lvl">
					<span class="level">[{{ log.lvl }}]</span>
					<span class="date">[{{ formatDate(new Date(log.time)) }}]</span>
					<span>:&nbsp;</span>
					<span class="log">{{ log.log }}</span>
				</div>
			</div>
		</div>
		<div class="shadow"></div>
	</div>
</template>
<style scoped>
.outer {
	position: relative;
	border-radius: 1rem;
	background: #555;
	overflow: auto;
}

.inner {
	width: 100%;
	height: 100%;
	padding: 1rem;
	overflow: auto;
}

.shadow {
	position: absolute;
	top: 0;
	left: 0;
	width: 100%;
	height: 100%;
	border-radius: 1rem;
	box-shadow: 3px 3px 20px -3px #000 inset;
	pointer-events: none;
}

.box {
	display: flex;
	flex-direction: column;
	font-size: 1rem;
	font-family: monospace;
	white-space: pre;
}

.line {
	line-height: 120%;
	color: #fff;
}

.line[level='DBUG'] {
	color: #009b00;
}

.line[level='INFO'] {
	color: #eee;
}

.line[level='WARN'] {
	color: #ffd500;
}

.line[level='ERRO'] {
	color: #ff1010;
}

.level {
	font-weight: bold;
}

.log {
	margin-right: 1rem;
}

@media (max-width: 60rem) {
	.box {
		font-size: 0.85rem;
	}
}
</style>
