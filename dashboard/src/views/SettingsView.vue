<script setup lang="ts">
import { ref, reactive, computed, watch, inject, onMounted, type Ref } from 'vue'
import { RouterLink } from 'vue-router'
import Card from 'primevue/card'
import Calendar from 'primevue/calendar'
import Dropdown from 'primevue/dropdown'
import InputIcon from 'primevue/inputicon'
import InputSwitch from 'primevue/inputswitch'
import { useToast } from 'primevue/usetoast'
import {
	getSubscribePublicKey,
	getSubscribeSettings,
	setSubscribeSettings,
	removeSubscription,
	type SubscribeScope,
} from '@/api/v0'
import { bindObjectToLocalStorage } from '@/cookies'
import { type Lang, avaliableLangs, getLang, setLang, tr, langNameMap } from '@/lang'

const toast = useToast()
const token = inject('token') as Ref<string | null>

const languages = avaliableLangs.map((v) => v.code)
const selectedLang = computed({
	get() {
		return getLang()
	},
	set(value) {
		setLang(value)
	},
})

const requestingPermission = ref(false)

const enableNotify = ref(false)
const settings = bindObjectToLocalStorage(
	{
		notifyWhenDisabled: false,
		notifyWhenEnabled: false,
		notifyWhenSyncBegin: false,
		notifyWhenSyncFinished: false,
		notifyUpdates: false,
		dailyReport: false,
		dailyReportAt: '00:00',
	},
	'go-openbmclapi.settings.notify',
)
const dailyReportAt = ref(
	new Date(
		0,
		0,
		0,
		parseInt(settings.dailyReportAt.substr(0, 2)),
		parseInt(settings.dailyReportAt.substr(3, 2)),
	),
)
watch(dailyReportAt, (v: Date) => {
	settings.dailyReportAt = `${v.getHours().toString().padStart(2, '0')}:${v
		.getMinutes()
		.toString()
		.padStart(2, '0')}`
})

function getSubscribeScopes(): SubscribeScope[] {
	const res: SubscribeScope[] = []
	if (settings.notifyWhenDisabled) {
		res.push('disabled')
	}
	if (settings.notifyWhenEnabled) {
		res.push('enabled')
	}
	if (settings.notifyWhenSyncBegin) {
		res.push('syncbegin')
	}
	if (settings.notifyWhenSyncFinished) {
		res.push('syncdone')
	}
	if (settings.notifyUpdates) {
		res.push('updates')
	}
	if (settings.dailyReport) {
		res.push('dailyreport')
	}
	return res
}

var pubKeyCache: string | null = null
var pubKeyLastUpd: number = 0
var subscribed: boolean = false

async function subscribe(): Promise<void> {
	if (!enableNotify.value) {
		return
	}
	const tk = token.value
	if (!tk) {
		console.error('Accessed settings without login')
		return
	}
	if (Date.now() - pubKeyLastUpd > 1000 * 60 * 60) {
		pubKeyCache = await getSubscribePublicKey()
		pubKeyLastUpd = Date.now()
		console.debug('subscribe application server key:', pubKeyCache)
	}
	var subs: PushSubscription | null = null
	if (!subscribed) {
		const sw = await navigator.serviceWorker.ready
		subs = await sw.pushManager.subscribe({
			userVisibleOnly: true,
			applicationServerKey: pubKeyCache,
		})
		console.debug('subscription:', subs.toJSON())
		subscribed = true
	}
	await setSubscribeSettings(tk, subs, getSubscribeScopes())
}

async function onEnableNotify(): Promise<void> {
	if (!token.value) {
		toast.add({
			severity: 'error',
			summary: tr('message.settings.login.first'),
			life: 5000,
		})
		return
	}
	if (enableNotify.value) {
		removeSubscription(token.value)
		enableNotify.value = false
		return
	}
	requestingPermission.value = true
	try {
		if ((await Notification.requestPermission()) !== 'granted') {
			toast.add({
				severity: 'error',
				summary: tr('message.settings.notify.cant.enable'),
				detail: tr('message.settings.notify.denied'),
				life: 5000,
			})
			return
		}
		const sw = await navigator.serviceWorker.ready
		switch (await sw.pushManager.permissionState({ userVisibleOnly: true })) {
			case 'granted':
				break
			case 'prompt':
				break
			default:
				toast.add({
					severity: 'error',
					summary: tr('message.settings.webpush.cant.enable'),
					detail: tr('message.settings.webpush.denied'),
					life: 5000,
				})
				return
		}
	} catch (e) {
		console.error('subscription failed:', e)
		toast.add({
			severity: 'error',
			summary: tr('message.settings.webpush.cant.enable'),
			detail: String(e),
			life: 5000,
		})
		return
	} finally {
		requestingPermission.value = false
	}
	enableNotify.value = true
	subscribe()
}

watch(settings, subscribe)

onMounted(() => {
	if (!token.value) {
		return
	}
	requestingPermission.value = true
	getSubscribeSettings(token.value)
		.then((sets) => {
			if (sets === null) {
				return
			}
			subscribed = true
			enableNotify.value = true
			settings.notifyWhenDisabled = sets.scopes.disabled
			settings.notifyWhenEnabled = sets.scopes.enabled
			settings.notifyWhenSyncBegin = sets.scopes.syncbegin
			settings.notifyWhenSyncFinished = sets.scopes.syncdone
			settings.notifyUpdates = sets.scopes.updates
			settings.dailyReport = sets.scopes.dailyreport
		})
		.finally(() => {
			requestingPermission.value = false
		})
})
</script>
<template>
	<div>
		<h1>
			<i class="pi pi-cog" style="font-size: 0.85em"></i>
			{{ tr('title.settings') }}
		</h1>
		<Card class="settings-group">
			<template #title>
				<div class="flex-row-center settings-group-title">
					<lable>{{ tr('title.i18n') }}</lable>
				</div>
			</template>
			<template #content>
				<div class="flex-row-center settings-elem">
					<lable class="settings-label">{{ tr('title.language') }}</lable>
					<Dropdown
						v-model="selectedLang"
						class="lang-selector"
						:options="languages"
						placeholder="Language"
					>
						<template #value="slotProps">
							<span class="flex-row-center lang-selector-label" style="margin-right: -0.75rem">
								<i class="pi pi-globe"></i>
								{{ langNameMap[slotProps.value.toString()] }}
							</span>
						</template>
						<template #option="slotProps">
							{{ langNameMap[slotProps.option.toString()] }}
						</template>
					</Dropdown>
				</div>
			</template>
		</Card>
		<Card class="settings-group">
			<template #title>
				<div class="flex-row-center settings-group-title">
					<lable>{{ tr('title.notification') }}</lable>
					<InputSwitch
						v-model="enableNotify"
						@click.prevent="onEnableNotify"
						:disabled="requestingPermission"
					/>
				</div>
			</template>
			<template #content>
				<div class="settings-elem">
					<lable class="settings-label">{{ tr('title.notify.when.disabled') }}</lable>
					<InputSwitch
						v-model="settings.notifyWhenDisabled"
						:disabled="requestingPermission || !enableNotify"
					/>
				</div>
				<div class="settings-elem">
					<lable class="settings-label">{{ tr('title.notify.when.enabled') }}</lable>
					<InputSwitch
						v-model="settings.notifyWhenEnabled"
						:disabled="requestingPermission || !enableNotify"
					/>
				</div>
				<div class="settings-elem">
					<lable class="settings-label">{{ tr('title.notify.when.sync.done') }}</lable>
					<InputSwitch
						v-model="settings.notifyWhenSyncFinished"
						:disabled="requestingPermission || !enableNotify"
					/>
				</div>
				<div class="settings-elem">
					<lable class="settings-label">{{ tr('title.notify.when.update.available') }}</lable>
					<InputSwitch
						v-model="settings.notifyUpdates"
						:disabled="requestingPermission || !enableNotify"
					/>
				</div>
				<div class="settings-elem">
					<lable class="settings-label">{{ tr('title.notify.report.daily') }}</lable>
					<InputSwitch
						v-model="settings.dailyReport"
						:disabled="requestingPermission || !enableNotify"
					/>
				</div>
				<div class="settings-elem">
					<lable class="settings-label" style="margin-left: 1.5rem">{{
						tr('title.notify.report.at')
					}}</lable>
					<Calendar
						class="time-input"
						v-model="dailyReportAt"
						showIcon
						iconDisplay="input"
						timeOnly
						:stepMinute="15"
					>
						<template #inputicon="{ clickCallback }">
							<InputIcon class="pi pi-clock pointer" @click="clickCallback" />
						</template>
					</Calendar>
				</div>
				<div class="settings-elem">
					<RouterLink to="/settings/notifications" style="color: inherit">
						{{ tr('title.notify.advanced') }}
					</RouterLink>
				</div>
			</template>
		</Card>
	</div>
</template>
<style>
.settings-group {
	width: 30rem;
	font-size: 1rem;
	margin-bottom: 2rem;
}

.settings-group-title {
	justify-content: space-between;
	width: 100%;
	padding: 0 1rem;
}

.settings-elem {
	display: flex;
	flex-direction: row;
	align-items: center;
	justify-content: space-between;
	width: 100%;
	padding: 0.4rem 1rem;
	background-color: var(--surface-c);
}

.settings-elem:nth-child(even) {
	background-color: var(--surface-d);
}

.settings-label {
	max-width: calc(100% - 3rem);
}

.lang-selector .pi-globe {
	margin-right: 0.3rem;
}

.lang-selector-label {
	font-size: 0.9rem;
}

.time-input {
	width: 7.5rem;
}

@media (max-width: 60rem) {
	.settings-group {
		width: 100%;
	}

	.settings-group > .p-card-body {
		padding-left: 0;
		padding-right: 0;
	}
}
</style>
