<script setup lang="ts">
import { onMounted, ref, computed, watch, inject, type Ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useRequest } from 'vue-request'
import Button from 'primevue/button'
import Chart from 'primevue/chart'
import ProgressSpinner from 'primevue/progressspinner'
import Skeleton from 'primevue/skeleton'
import Message from 'primevue/message'
import InputSwitch from 'primevue/inputswitch'
import { useToast } from 'primevue/usetoast'
import { formatNumber, formatBytes, formatTime } from '@/utils'
import HitsChart from '@/components/HitsChart.vue'
import UAChart from '@/components/UAChart.vue'
import LogBlock from '@/components/LogBlock.vue'
import StatusButton from '@/components/StatusButton.vue'
import { getStatus, getPprofURL, type StatInstData, type PprofLookups } from '@/api/v0'
import { LogIO, type LogMsg } from '@/api/log.io'
import { bindRefToLocalStorage } from '@/cookies'
import { tr } from '@/lang'

const toast = useToast()
const token = inject('token') as Ref<string | null>

const logBlk = ref<InstanceType<typeof LogBlock>>()

const requestingPprof = ref(false)
const logDebugLevel = bindRefToLocalStorage(ref(false), 'dashboard.log.debug.bool')

const now = ref(new Date())
setInterval(() => {
	now.value = new Date()
}, 1000)

const { data, error, loading } = useRequest(getStatus, {
	pollingInterval: 10000,
	loadingDelay: 500,
	loadingKeep: 2000,
})
error.value = 'Loading ...'

var requestingLogIO = false
var logIO: LogIO | null = null

watch(
	() => [data.value, error.value],
	() => {
		if (!error.value && !(requestingLogIO || logIO) && token.value) {
			connectLogIO(token.value)
		}
	},
)

const status = computed(() =>
	error.value ? 'error' : data.value && data.value.enabled ? 'enabled' : 'disabled',
)

const stat = computed(() => {
	if (!data.value) {
		return
	}
	const stat = data.value.stats
	stat.days = cutDays(stat.days, stat.date.year, stat.date.month)
	stat.prev.days = cutDays(stat.prev.days, stat.date.year, stat.date.month - 1)

	stat.days[stat.date.day] = stat.hours.reduce((sum, v) => ({
		hits: sum.hits + v.hits,
		bytes: sum.bytes + v.bytes,
	}))
	stat.months[stat.date.month] = stat.days.reduce((sum, v) => ({
		hits: sum.hits + v.hits,
		bytes: sum.bytes + v.bytes,
	}))
	stat.years[stat.date.year.toString()] = stat.months.reduce((sum, v) => ({
		hits: sum.hits + v.hits,
		bytes: sum.bytes + v.bytes,
	}))
	return stat
})

function formatHour(hour: number): string {
	const offset = -new Date().getTimezoneOffset()
	let min = hour * 60 + offset
	hour = Math.floor(min / 60) % 24
	min %= 60
	if (hour < 0) {
		hour += 24
	}
	if (min < 0) {
		min += 60
	}
	return `${hour}:${min.toString().padStart(2, '0')}`
}

function cutDays(days: StatInstData[], year: number, month: number): StatInstData[] {
	const dayCount = new Date(year, month, 0).getDate()
	days.length = dayCount
	return days
}

function formatDay(day: number): string {
	if (!stat.value) {
		return ''
	}
	const date = new Date(Date.UTC(stat.value.date.year, stat.value.date.month, day))
	return `${date.getMonth() + 1}-${date.getDate()}`
}

function formatMonth(month: number): string {
	if (!stat.value) {
		return ''
	}
	const date = new Date(Date.UTC(stat.value.date.year, month + 1, 1))
	return `${date.getFullYear()}-${(date.getMonth() + 1).toString().padStart(2, '0')}`
}

function getDaysInMonth(): number {
	const date = new Date()
	const days = new Date(date.getFullYear(), date.getMonth() + 1, 0).getDate()
	return date.getDate() / days
}

async function requestPprof(lookup: PprofLookups, view?: boolean): Promise<void> {
	if (!token.value || requestingPprof.value) {
		return
	}
	requestingPprof.value = true
	const target = await getPprofURL(token.value, {
		lookup: lookup,
		view: view,
		debug: true,
	})
		.catch((err) => {
			console.error('Request pprof error:', err)
			toast.add({
				severity: 'error',
				summary: `Request pprof (${lookup}) error`,
				detail: String(err),
				life: 5000,
			})
			return null
		})
		.finally(() => (requestingPprof.value = false))
	if (!target) {
		return
	}
	window.open(target)
}

async function onTokenChanged(tk: string | null): Promise<void> {
	if (logIO) {
		logIO.close()
		logIO = null
	}
	if (!tk) {
		return
	}
	return connectLogIO(tk)
}

async function connectLogIO(tk: string): Promise<void> {
	if (requestingLogIO || logIO) {
		return
	}
	requestingLogIO = true

	logBlk.value?.pushLog({
		time: Date.now(),
		lvl: 'INFO',
		log: '[dashboard]: Connecting to remote server ...',
	})
	logIO = await LogIO.dial(tk).catch((err) => {
		console.error('Cannot connect to log.io:', err)
		logBlk.value?.pushLog({
			time: Date.now(),
			lvl: 'ERRO',
			log: '[dashboard]: Cannot connect to remote server: ' + String(err),
		})
		return null
	})
	requestingLogIO = false

	if (!logIO) {
		return
	}
	logIO.setLevel(logDebugLevel.value ? 'DBUG' : 'INFO')
	const unwatchDebugLevel = watch(logDebugLevel, (debug) => {
		logIO?.setLevel(debug ? 'DBUG' : 'INFO')
	})

	logBlk.value?.pushLog({
		time: Date.now(),
		lvl: 'INFO',
		log: '[dashboard]: Connected to remote server',
	})
	logIO.addCloseListener(() => {
		console.warn('log.io closed')
		unwatchDebugLevel()
		logBlk.value?.pushLog({
			time: Date.now(),
			lvl: 'ERRO',
			log: '[dashboard]: Disconnected from remote server',
		})
		logIO = null
		setTimeout(() => {
			if (!logIO && token.value) {
				connectLogIO(token.value)
			}
		}, 100)
	})
	logIO.addLogListener((msg: LogMsg) => {
		if (logDebugLevel.value || msg.lvl !== 'DBUG') {
			logBlk.value?.pushLog(msg)
		}
	})
}

onMounted(() => {
	onTokenChanged(token.value)
	watch(token, onTokenChanged)
})
</script>

<template>
	<main>
		<h1>Go-OpenBmclAPI {{ tr('title.dashboard') }}</h1>
		<div class="main">
			<div class="flex-row-center basic-info">
				<div class="flex-row-center" style="height: 4rem">
					<StatusButton :status="status" />
					<ProgressSpinner v-if="loading" class="polling" strokeWidth="6" />
				</div>
				<div v-if="error">
					<b>{{ error }}</b>
				</div>
				<template v-else-if="data">
					<div class="no-select">
						<span>{{ tr('message.server.run-for') }}&nbsp;</span>
						<span class="info-uptime">
							{{ formatTime(now.getTime() - new Date(data.startAt).getTime()) }}
						</span>
					</div>
					<div v-if="data.isSync" class="no-select">
						&nbsp; |
						{{ tr('message.server.synchronizing') }}
						&nbsp;
						<i>
							(
							<b>{{ data.sync?.prog }}</b>
							/
							<b>{{ data.sync?.total }}</b>
							)
						</i>
					</div>
				</template>
			</div>
			<div class="hits-chart-box">
				<div class="chart-card">
					<h3>{{ tr('title.day') }}</h3>
					<HitsChart
						v-if="stat"
						class="hits-chart"
						:max="25"
						:offset="23"
						:data="stat.hours"
						:oldData="stat.prev.hours"
						:current="stat.date.hour + new Date().getMinutes() / 60"
						:formatXLabel="formatHour"
					/>
					<Skeleton v-else width="" height="" class="hits-chart" />
				</div>
				<div class="chart-card">
					<h3>{{ tr('title.month') }}</h3>
					<HitsChart
						v-if="stat"
						class="hits-chart"
						:max="31"
						:offset="29"
						:data="stat.days"
						:oldData="stat.prev.days"
						:current="stat.date.day + new Date().getUTCHours() / 24"
						:formatXLabel="formatDay"
					/>
					<Skeleton v-else width="" height="" class="hits-chart" />
				</div>
				<div class="chart-card">
					<h3>{{ tr('title.year') }}</h3>
					<HitsChart
						v-if="stat"
						class="hits-chart"
						:max="13"
						:offset="11"
						:data="stat.months"
						:oldData="stat.prev.months"
						:current="stat.date.month + getDaysInMonth()"
						:formatXLabel="formatMonth"
					/>
					<Skeleton v-else width="" height="" class="hits-chart" />
				</div>
				<!-- TODO: show yearly chart -->
			</div>
			<div class="info-chart-box">
				<h3>{{ tr('title.user_agents') }}</h3>
				<UAChart v-if="stat" class="ua-chart" :max="5" :data="stat.accesses" />
				<Skeleton v-else width="" height="" class="ua-chart" />
			</div>
		</div>
		<div class="log-box">
			<template v-if="token">
				<nav class="pprof-nav">
					<Button
						severity="warning"
						:label="tr('title.pprof.heap')"
						:loading="requestingPprof"
						@click="(e) => requestPprof('heap', e.shiftKey)"
					/>
					<Button
						severity="primary"
						:label="tr('title.pprof.goroutine')"
						:loading="requestingPprof"
						@click="(e) => requestPprof('goroutine', e.shiftKey)"
					/>
					<Button
						severity="contrast"
						:label="tr('title.pprof.allocs')"
						:loading="requestingPprof"
						@click="(e) => requestPprof('allocs', e.shiftKey)"
					/>
					<Button
						severity="info"
						:label="tr('title.pprof.block')"
						:loading="requestingPprof"
						@click="(e) => requestPprof('block', e.shiftKey)"
					/>
					<Button
						severity="help"
						:label="tr('title.pprof.mutex')"
						:loading="requestingPprof"
						@click="(e) => requestPprof('mutex', e.shiftKey)"
					/>
				</nav>
				<div class="flex-row-center log-options">
					<div class="flex-row-center">
						<span class="no-select">{{ tr('message.log.option.debug') }}&nbsp;</span>
						<InputSwitch v-model="logDebugLevel" />
					</div>
				</div>
				<LogBlock ref="logBlk" class="log-block" />
			</template>
			<Message v-else :closable="false" severity="info">
				<RouterLink to="/login" style="color: inherit">
					{{ tr('title.login') }}
				</RouterLink>
				<span>{{ tr('message.log.login-to-view') }}</span>
			</Message>
		</div>
	</main>
</template>
<style scoped>
* {
	margin: 0;
}

.main {
	display: grid;
	grid-template:
		'a a' 4rem
		'b c' auto
		/ 46rem auto;
	grid-gap: 1rem;
}

.basic-info {
	grid-area: a;
	height: 4rem;
	font-weight: 200;
}

.hits-chart-box {
	grid-area: b;
}

.info-chart-box {
	grid-area: c;
}

.polling {
	width: 1.5rem;
	margin-right: 0.2rem;
}

.info-uptime {
	font-weight: 700;
	font-style: italic;
}

.chart-card {
	margin-bottom: 1rem;
}

.hits-chart {
	max-width: 100%;
	width: 45rem;
	height: 13rem;
	user-select: none;
}

.ua-chart {
	width: 25rem;
	height: 13rem;
	user-select: none;
}

.log-box {
	margin-top: 2rem;
}

.pprof-nav {
	display: inline-flex;
	flex-direction: row;
}

.log-options {
	display: inline-flex;
	margin-top: 1rem;
}

.pprof-nav > *,
.log-options > div {
	margin-right: 1rem;
}

.log-block {
	margin-top: 1rem;
	height: calc(100vh - 12rem);
}

@media (max-width: 60rem) {
	.main {
		display: flex;
		flex-direction: column;
	}

	.basic-info {
		flex-direction: column;
		align-items: flex-start;
		height: unset;
	}

	.hits-chart,
	.ua-chart {
		width: 100%;
	}

	.pprof-nav {
		display: grid;
		grid-template:
			'a a' 2.5rem
			'b c' 2.5rem
			'd e' 2.5rem
			/ calc(50%) calc(50%);
		grid-gap: 0.5rem;
	}

	.pprof-nav > *:first-child {
		grid-area: a;
	}

	.pprof-nav > * {
		height: 2.5rem;
		margin: 0;
		font-size: 0.85rem;
		white-space: pre;
	}

	.log-block {
		height: calc(100vh - 18rem);
	}
}
</style>
