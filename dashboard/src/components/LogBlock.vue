<script setup lang="ts">
import { ref, reactive, nextTick, getCurrentInstance, onMounted, onBeforeUnmount } from 'vue'
import { RingBuffer } from '@/utils/ring'

interface Log {
	_inc?: number
	time: number
	lvl: string
	log: string
}

const MAX_LOG_LENGTH = 1024 * 2

const box = ref<HTMLElement>()
const logs = reactive(RingBuffer.create<Log>(MAX_LOG_LENGTH))

var logInc = 0
var focusLastLog = true
var justSplicedLog = false
var logDelayWatcher: Promise<void> | null = null

var scrollYTarget: number | null = null
var scrolling = false

function registerAnimationFrame(callback: (dt: number) => void | boolean): () => boolean {
	var n: ReturnType<typeof window.requestAnimationFrame> | null = null
	var last: number = document.timeline.currentTime as number
	const step = (ts: number) => {
		n = null
		const dt = ts - last
		last = ts
		if (callback(dt) === false) {
			return
		}
		n = window.requestAnimationFrame(step)
	}
	n = window.requestAnimationFrame(step)
	return () => {
		if (n === null) {
			return true
		}
		window.cancelAnimationFrame(n)
		n = null
		return false
	}
}

function activeScroller(): void {
	if (scrolling) {
		return
	}
	scrolling = true

	const MIN_SCROLL_SPEED = 500 // 500px per second
	registerAnimationFrame((dt: number): boolean => {
		if (!focusLastLog || !scrollYTarget || document.hidden || !box.value) {
			scrolling = false
			return false
		}
		const diff = scrollYTarget - (box.value.scrollTop + box.value.clientHeight)
		const minDist = (dt / 1000) * MIN_SCROLL_SPEED
		if (diff <= minDist) {
			box.value.scrollTop = scrollYTarget - box.value.clientHeight
			scrolling = false
			return false
		}
		box.value.scrollTop =
			scrollYTarget - box.value.clientHeight - diff + Math.max(diff * 0.1, minDist)
		return true
	})
}

function pushLog(log: Log): void {
	log._inc = logInc = (logInc + 1) % 65536
	logs.push(log)
	justSplicedLog = true
	if (justSplicedLogCleaner) {
		window.cancelAnimationFrame(justSplicedLogCleaner)
		justSplicedLogCleaner = null
	}

	if (!logDelayWatcher) {
		logDelayWatcher = nextTick().then(() => {
			logDelayWatcher = null
			if (document.hidden || !box.value) {
				return
			}
			if (focusLastLog) {
				scrollYTarget = box.value.scrollHeight
				activeScroller()
			}
		})
	}
}

var boxLastPosTop = 0
var justSplicedLogCleaner: ReturnType<typeof window.requestAnimationFrame> | null = null

function onScrollBox(): void {
	if (document.hidden || !box.value) {
		return
	}
	const scrolledY = box.value.scrollTop - boxLastPosTop
	boxLastPosTop = box.value.scrollTop
	if (scrolledY < 0) {
		if (justSplicedLog) {
			if (!justSplicedLogCleaner) {
				justSplicedLogCleaner = window.requestAnimationFrame(() => {
					justSplicedLogCleaner = null
					justSplicedLog = false
				})
			}
		} else {
			focusLastLog = false
		}
	} else {
		const diff = box.value.scrollHeight - (box.value.scrollTop + box.value.clientHeight)
		if (diff <= 5) {
			focusLastLog = true
		}
	}
}

function onVisibilityChange(): void {
	if (document.hidden || !box.value || !focusLastLog) {
		return
	}
	scrollYTarget = box.value.scrollHeight
	activeScroller()
}

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
