<script setup lang="ts">
import { onMounted, ref, computed, watch, inject, type Ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useRequest } from 'vue-request'
import Button from 'primevue/button'
import InputSwitch from 'primevue/inputswitch'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Skeleton from 'primevue/skeleton'
import TabMenu from 'primevue/tabmenu'
import { useToast } from 'primevue/usetoast'
import { formatTime } from '@/utils'
import HitsCharts from '@/components/HitsCharts.vue'
import UAChart from '@/components/UAChart.vue'
import LogBlock from '@/components/LogBlock.vue'
import StatusButton from '@/components/StatusButton.vue'
import {
	getStatus,
	getStat,
	getPprofURL,
	EMPTY_STAT,
	type Stats,
	type StatusRes,
	type PprofLookups,
} from '@/api/v0'
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

const { data, error, loading } = useRequest((): Promise<StatusRes> => getStatus(token.value), {
	pollingInterval: 10000,
	loadingDelay: 500,
	loadingKeep: 2000,
})
error.value = new Error('Loading ...')

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

const OVERALL_ID = ':Overall:'
const avaliableStorages = computed(() => {
	const s = [{ label: OVERALL_ID }]
	if (data.value) {
		for (const storage of data.value.storages) {
			s.push({ label: storage })
		}
	}
	return s
})
const activeStorageIndex = ref(0)
const activeStats = ref<Stats | null>(null)

watch(activeStorageIndex, async (newIndex) => {
	if (!data.value) {
		activeStats.value = null
		return
	}
	if (!newIndex) {
		activeStats.value = data.value.stats
		return
	}
	const storageId = data.value.storages[newIndex - 1]
	if (!storageId) {
		activeStorageIndex.value = 0
		activeStats.value = null
		return
	}
	const statClearTimerId = setTimeout(() => {
		activeStats.value = null
	}, 500)
	const res = await getStat(storageId, token.value)
	clearTimeout(statClearTimerId)
	if (res === null) {
		activeStats.value = JSON.parse(JSON.stringify(EMPTY_STAT))
		return
	}
	activeStats.value = res
	return
})

watch(data, async (data) => {
	if (!data) {
		activeStats.value = null
		return
	}
	const newIndex = activeStorageIndex.value
	if (!newIndex) {
		activeStats.value = data.stats
		return
	}
	const storageId = data.storages[newIndex - 1]
	if (!storageId) {
		activeStorageIndex.value = 0
		activeStats.value = null
		return
	}
	const res = await getStat(storageId, token.value)
	if (res === null) {
		activeStats.value = JSON.parse(JSON.stringify(EMPTY_STAT))
		return
	}
	activeStats.value = res
	return
})

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
					<div class="select-none">
						<span>{{ tr('message.server.run-for') }}&nbsp;</span>
						<span class="info-uptime">
							{{ formatTime(now.getTime() - new Date(data.startAt).getTime()) }}
						</span>
					</div>
					<div v-if="data.isSync" class="select-none">
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
			<div class="charts-tab">
				<TabMenu
					style="
						border-top-left-radius: var(--border-radius);
						border-top-right-radius: var(--border-radius);
					"
					:model="avaliableStorages"
					v-model:activeIndex="activeStorageIndex"
				/>
				<HitsCharts class="charts-tab-charts" :stats="activeStats" />
			</div>
			<div class="info-chart-box">
				<h3>{{ tr('title.user_agents') }}</h3>
				<UAChart v-if="data && data.stats" class="ua-chart" :max="5" :data="data.stats.accesses" />
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
						<span class="select-none">{{ tr('message.log.option.debug') }}&nbsp;</span>
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

.charts-tab {
	border: 1px solid var(--surface-border);
	border-radius: var(--border-radius);
}

.charts-tab-charts {
	margin: 0.5rem;
	margin-top: 0;
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

	.charts-tab {
		position: relative;
		left: -1.5rem;
		width: calc(100% + 3rem);
	}

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
