<script setup lang="ts">
import { ref, reactive, watch, nextTick } from 'vue'

interface Log {
	_inc?: number
	time: number
	lvl: string
	log: string
}

const box = ref<HTMLElement>()
const logs = reactive<Log[]>([])
const MAX_LOG_LENGTH = 1000

var logInc = 0
var focusLastLog = true

function pushLog(log: Log) {
	log._inc = (logInc = (logInc + 1) % 65536)
	logs.push(log)
	const minI = logs.length - MAX_LOG_LENGTH
	if (minI > 0) {
		logs.splice(0, minI)
	}
}

var boxLastPostion = 0

function onScrollBox() {
	if(!box.value){
		return
	}
	const scrolled = box.value.scrollTop - boxLastPostion
	boxLastPostion = box.value.scrollTop
	if(scrolled < 0){
		focusLastLog = false
	}
}

watch(logs, async (logs) => {
	if(document.hidden || !box.value){
		return
	}
	console.log('logs:', logs.length)
	const diff = box.value.scrollHeight - box.value.scrollTop + box.value.clientHeight
	focusLastLog ||= diff < 5
	if(focusLastLog){
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

</script>
<template>
	<div ref="box" class="box" @scroll="onScrollBox">
		<div
			v-for="log in logs"
			:key="`${log.time},${log._inc}`"
			class="line"
			:level="log.lvl"
		>
			<span class="level">[{{log.lvl}}]</span>
			<span class="date">[{{formatDate(new Date(log.time))}}]</span>
			<span>:&nbsp;</span>
			<span class="log">{{log.log}}</span>
		</div>
	</div>	
</template>
<style scoped>
.box {
	display: flex;
	flex-direction: column;
	padding: 1rem;
	border-radius: 1rem;
  font-size: 1rem;
	font-family: monospace;
	background: #555;
	white-space: pre;
	overflow: auto;
}

.line {
  line-height: 120%;
  color: #fff;
}

.line[level=DBUG] {
	color: #009b00;
}

.line[level=INFO] {
	color: #eee;
}

.line[level=WARN] {
	color: #ffd500;
}

.line[level=ERRO] {
	color: #ff1010;
}

.level {
	font-weight: bold;
}
</style>