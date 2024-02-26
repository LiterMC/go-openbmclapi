<script setup lang="ts">
import { reactive, watch } from 'vue'
import Card from 'primevue/card'
import InputSwitch from 'primevue/inputswitch'
import { useToast } from 'primevue/usetoast'
import { tr } from '@/lang'

const toast = useToast()

const settings = reactive({
	enableNotify: false,
	notifyWhenDisabled: false,
	notifyWhenEnabled: false,
	notifyWhenSyncFinished: false,
	notifyUpdates: false,
})

watch(
	() => settings.enableNotify,
	(notify) => {
		if (notify) {
			Notification.requestPermission().then((perm) => {
				if (perm !== 'granted') {
					settings.enableNotify = false
					toast.add({
						severity: 'error',
						summary: tr('message.settings.notify.cant.enable'),
						detail: tr('message.settings.notify.denied'),
						life: 5000,
					})
				}
			})
		}
	},
)
</script>
<template>
	<div>
		<h1>{{ tr('title.settings') }}</h1>
		<Card class="settings-group">
			<template #title>
				<div class="flex-row-center settings-group-title">
					<lable>{{ tr('title.notification') }}</lable>
					<InputSwitch v-model="settings.enableNotify" />
				</div>
			</template>
			<template #content>
				<div class="flex-row-center settings-elem">
					<lable class="settings-label">{{ tr('title.notify.when.disabled') }}</lable>
					<InputSwitch v-model="settings.notifyWhenDisabled" :disabled="!settings.enableNotify" />
				</div>
				<div class="flex-row-center settings-elem">
					<lable class="settings-label">{{ tr('title.notify.when.enabled') }}</lable>
					<InputSwitch v-model="settings.notifyWhenEnabled" :disabled="!settings.enableNotify" />
				</div>
				<div class="flex-row-center settings-elem">
					<lable class="settings-label">{{ tr('title.notify.when.sync.done') }}</lable>
					<InputSwitch
						v-model="settings.notifyWhenSyncFinished"
						:disabled="!settings.enableNotify"
					/>
				</div>
				<div class="flex-row-center settings-elem">
					<lable class="settings-label">{{ tr('title.notify.when.update.available') }}</lable>
					<InputSwitch v-model="settings.notifyUpdates" :disabled="!settings.enableNotify" />
				</div>
			</template>
		</Card>
	</div>
</template>
<style>
.settings-group {
	width: 30rem;
	font-size: 1rem;
}

.settings-group-title {
	justify-content: space-between;
	width: 100%;
	padding: 0 1rem;
}

.settings-elem {
	justify-content: space-between;
	width: 100%;
	padding: 0.4rem 1rem;
	background-color: var(--surface-c);
}

.settings-elem:nth-child(even) {
	background-color: var(--surface-d);
}

.settings-label {
	width: calc(100% - 3rem);
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
