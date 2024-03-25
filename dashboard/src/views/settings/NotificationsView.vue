<script setup lang="ts">
import { ref, computed, nextTick } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import MultiSelect from 'primevue/multiselect'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import TransitionExpandGroup from '@/components/TransitionExpandGroup.vue'
import { ALL_SUBSCRIBE_SCOPES, type SubscribeScope } from '@/api/v0'
import { tr } from '@/lang'

interface EmailItemRes {
	addr: string
	enabled: boolean
}

interface WebhookItemPayload {
	name: string
	endpoint: string
	auth: string
	scopes: SubscribeScope[]
}

interface WebhookItemRes {
	id: number
	name: string
	endpoint: string
	enabled: boolean
	authHash?: string
	scopes: SubscribeScope[]
}

const emails = ref<EmailItemRes[]>([
	{ addr: 'test@example.com', enabled: true },
	{ addr: 'zyxkad@gmail.com', enabled: true },
])
const newEmailItem = ref('')
const newEmailItemInvalid = ref<string | null>(null)

const webhookEditingItem = ref<(WebhookItemPayload & { _: WebhookItemRes }) | null>(null)
const webhookEdited = computed((): boolean => {
	const item = webhookEditingItem.value
	if (!item) {
		return true
	}
	const ori = item._
	for (const k of ['name', 'endpoint'] as const) {
		if (ori[k] !== item[k]) {
			return true
		}
	}
	if (ori.scopes.join() !== item.scopes.join()) {
		return true
	}
	if (item.auth !== '-') {
		// '-' is the placeholder represent that the auth header was not modified
		return true
	}
	return false
})
const webhookEditSaving = ref(false)
const webhookEditNameInvalid = ref<string | null>(null)
const webhookEditEndpointInvalid = ref<string | null>(null)
const webhookEditAuthInvalid = ref<string | null>(null)

const webhooks = ref<WebhookItemRes[]>([
	{
		id: 0,
		name: 'Example Webhook',
		endpoint: 'https://example.com/endpoint',
		enabled: true,
		authHash: 'sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad',
		scopes: ['enabled', 'disabled'],
	},
	{
		id: 1,
		name: 'Another Webhook',
		endpoint: 'https://another.example.com/endpoint2',
		enabled: true,
		authHash: 'sha256:18ac3e7343f016890c510e93f935261169d9e3f565436429830faf0934f4f8e4',
		scopes: ['enabled', 'disabled', 'syncdone', 'updates', 'dailyreport'],
	},
	{
		id: 2,
		name: 'Disabled Webhook',
		endpoint: 'https://example.com/disabled',
		enabled: false,
		scopes: ['enabled', 'syncdone', 'dailyreport'],
	},
])

function checkEmail(email: string): string | null {
	if (!email.match(/^[\w-\.]+@([\w-]+\.)+[\w-]{2,}$/)) {
		return 'Email is invalid'
	}
	return null
}

async function removeEmail(index: number): Promise<void> {
	emails.value.splice(index, 1)
}

async function addEmail(addr: string): Promise<void> {
	emails.value.push({ addr: addr, enabled: true })
}

function openWebhookEditDialog(item: WebhookItemRes): void {
	webhookEditingItem.value = {
		...item,
		auth: '-',
		_: item,
	}
	webhookEditNameInvalid.value = null
	webhookEditEndpointInvalid.value = null
	webhookEditAuthInvalid.value = null
}

function checkName(name: string): string | null {
	if (name.length >= 32) {
		return 'Name must be less than 32 characters'
	}
	return null
}

function checkEndPoint(url: string): string | null {
	try {
		const u = new URL(url)
		if (!url.match(/^\s*http(?:s)?:\/\//) || (u.protocol !== 'http:' && u.protocol !== 'https:')) {
			return 'EndPoint must use http or https protocol'
		}
	} catch (e) {
		return 'Invalid URL'
	}
	return null
}

function checkAuth(auth: string | undefined): string | null {
	if (!auth) {
		return null
	}
	if (auth.length >= 256) {
		return 'Auth header must be less than 256 characters'
	}
	return null
}

async function webhookEditSave(): Promise<void> {
	if (!webhookEditingItem.value) {
		console.warn('webhookEditingItem is null')
		return
	}
	webhookEditNameInvalid.value = null
	webhookEditEndpointInvalid.value = null
	webhookEditAuthInvalid.value = null
	webhookEditSaving.value = true
	try {
		const { name, endpoint, auth } = webhookEditingItem.value
		if ((webhookEditNameInvalid.value = checkName(name))) {
			return
		}
		if ((webhookEditEndpointInvalid.value = checkEndPoint(endpoint))) {
			return
		}
		if ((webhookEditAuthInvalid.value = checkAuth(auth))) {
			return
		}
		await new Promise((re) => setTimeout(re, 3000))
		webhookEditingItem.value = null
	} finally {
		webhookEditSaving.value = false
	}
}
</script>
<template>
	<div class="flex-row-center">
		<RouterLink to="/settings" class="pi pi-arrow-left" />
		<h1>{{ tr('title.notification') }}</h1>
	</div>
	<div class="section">
		<div class="flex-row-center section-title">
			<h2>{{ tr('title.emails') }}</h2>
		</div>
		<Card class="email-box">
			<template #content>
				<DataTable :value="emails" scrollable scrollHeight="min(30rem, 70vh)">
					<Column :header="tr('title.email.addr')" field="addr">
						<template #footer>
							<InputText
								type="email"
								autocomplete="email"
								placeholder="youremail@example.com"
								v-model="newEmailItem"
								:invalid="newEmailItem !== '' && newEmailItemInvalid !== null"
								@input="newEmailItemInvalid = checkEmail(newEmailItem)"
								style="width: 16rem; height: 2.6rem"
							/>
						</template>
					</Column>
					<Column :header="tr('title.operation')" rowEditor style="width: 8rem">
						<template #body="{ index, editorInitCallback }">
							<div class="flex-row-center">
								<Button
									class="op-button"
									size="small"
									aria-label="Edit"
									icon="pi pi-pencil"
									severity="secondary"
									text
									rounded
									style="margin-left: 0"
									:disabled="true"
									@click="editorInitCallback"
								/>
								<Button
									class="op-button"
									size="small"
									aria-label="Remove"
									icon="pi pi-trash"
									severity="danger"
									text
									rounded
									@click="removeEmail(index)"
								/>
							</div>
						</template>
						<template #footer>
							<Button
								class="op-button"
								size="small"
								aria-label="Add"
								icon="pi pi-plus"
								outlined
								style="float: right; margin-right: 0.6rem"
								@click="addEmail(newEmailItem)"
							/>
						</template>
					</Column>
				</DataTable>
			</template>
		</Card>
	</div>
	<div class="section">
		<div class="flex-row-center section-title">
			<h2>{{ tr('title.webhooks') }}</h2>
			<Button class="title-button" label="Create New" icon="pi pi-plus" outlined />
		</div>
		<div class="webhook-box">
			<Card v-for="item in webhooks" :key="item.id" class="margin-1">
				<template #title>
					<div class="flex-row-center flex-justify-end">
						<h4 class="secondary-title">{{ item.name }}</h4>
						<Button
							class="enable-button"
							:label="tr(item.enabled ? 'button.disable' : 'button.enable')"
							:severity="item.enabled ? 'danger' : 'info'"
							:outlined="item.enabled"
						/>
						<Button
							class="op-button"
							size="small"
							aria-label="Edit"
							icon="pi pi-wrench"
							severity="secondary"
							text
							rounded
							@click="openWebhookEditDialog(item)"
						/>
						<Button
							class="op-button"
							size="small"
							aria-label="Remove"
							icon="pi pi-trash"
							severity="danger"
							text
							rounded
						/>
					</div>
				</template>
				<template #content>
					<div class="flex-row-center text-overflow-ellipsis">
						<label class="webhook-edit-label">{{ tr('title.webhook.endpoint') }}:</label>
						<span class="flex-auto text-nowrap text-overflow-ellipsis">{{ item.endpoint }}</span>
					</div>
					<div class="flex-row-center text-overflow-ellipsis">
						<label class="webhook-edit-label">{{ tr('title.webhook.auth-header') }}:</label>
						<code
							v-if="item.authHash"
							class="flex-auto text-nowrap text-overflow-ellipsis font-monospace-09 select-all"
						>
							{{ item.authHash }}
						</code>
						<i v-else class="flex-auto"> N/A </i>
					</div>
					<div class="flex-row-center text-overflow-ellipsis">
						<label class="webhook-edit-label">{{ tr('title.webhook.scopes') }}:</label>
						<span class="flex-auto text-nowrap text-overflow-ellipsis">{{
							item.scopes.join(', ')
						}}</span>
					</div>
				</template>
			</Card>
		</div>
		<Dialog
			:visible="webhookEditingItem !== null"
			@update:visible="webhookEditingItem = null"
			modal
			style="width: 40rem"
		>
			<template #header>
				<div>
					<h3 class="font-bold margin-none">{{ tr('title.edit') }}</h3>
				</div>
			</template>
			<template v-if="webhookEditingItem">
				<div class="p-text-secondary margin-1">
					{{ tr('title.webhook.configure') }} "{{ webhookEditingItem._.name }}"
				</div>
				<div class="input-box">
					<div class="flex-row-center">
						<label class="webhook-edit-label">{{ tr('title.webhook.name') }}</label>
						<InputText
							class="flex-auto"
							autocomplete="off"
							v-model="webhookEditingItem.name"
							:invalid="webhookEditNameInvalid !== null"
							@input="webhookEditNameInvalid = checkName(webhookEditingItem.name)"
						/>
					</div>
					<p v-if="webhookEditNameInvalid" class="p-error">
						{{ webhookEditNameInvalid }}
					</p>
				</div>
				<div class="input-box">
					<div class="flex-row-center">
						<label class="webhook-edit-label">{{ tr('title.webhook.endpoint') }}</label>
						<InputText
							class="flex-auto"
							autocomplete="off"
							type="url"
							v-model="webhookEditingItem.endpoint"
							:invalid="webhookEditEndpointInvalid !== null"
							@input="webhookEditEndpointInvalid = checkEndPoint(webhookEditingItem.endpoint)"
						/>
					</div>
					<p v-if="webhookEditEndpointInvalid" class="p-error">
						{{ webhookEditEndpointInvalid }}
					</p>
				</div>
				<div class="input-box">
					<div class="flex-row-center">
						<label class="webhook-edit-label">{{ tr('title.webhook.auth-header') }}</label>
						<InputText
							class="flex-auto"
							autocomplete="off"
							v-model="webhookEditingItem.auth"
							:invalid="webhookEditAuthInvalid !== null"
							@input="webhookEditAuthInvalid = checkAuth(webhookEditingItem.auth)"
						/>
					</div>
					<p v-if="webhookEditAuthInvalid" class="p-error">
						{{ webhookEditAuthInvalid }}
					</p>
				</div>
				<div class="flex-row-center margin-1">
					<label class="webhook-edit-label">{{ tr('title.webhook.scopes') }}</label>
					<MultiSelect
						class="flex-auto"
						:options="ALL_SUBSCRIBE_SCOPES"
						v-model="webhookEditingItem.scopes"
					/>
				</div>
			</template>
			<template #footer>
				<Button
					label="Cancel"
					text
					severity="danger"
					@click="webhookEditingItem = null"
					autofocus
				/>
				<Button
					label="Save"
					icon="pi pi-download"
					outlined
					:severity="webhookEdited ? 'primary' : 'secondary'"
					:disabled="!webhookEdited"
					:loading="webhookEditSaving"
					@click="webhookEditSave()"
				/>
			</template>
		</Dialog>
	</div>
</template>
<style scoped>
h1,
h2 {
	margin: 0 1rem;
}

.section {
	margin-top: 1.2rem;
}

.section-title {
	margin-bottom: 1rem;
}

.title-button {
	height: 2.2rem;
	padding: 0 0.7rem;
}

.secondary-title {
	flex-grow: 1;
	margin: 0;
	font-size: 1.5rem;
}

.enable-button {
	display: flex;
	flex-direction: row;
	align-items: center;
	justify-content: center;
	height: 2.2rem;
	width: 5.6rem;
}

.op-button {
	width: 2.2rem;
	height: 2.2rem;
	margin-left: 0.8rem;
}

.email-box,
.webhook-box {
	max-width: 52rem;
}

.input-box {
	margin-bottom: 1rem;
}

.input-box > .p-error {
	margin: 0 0 1rem 7rem;
	font-size: 0.8rem;
}

.webhook-edit-label {
	display: block;
	width: 7rem;
	flex-shrink: 0;
	font-weight: bold;
	white-space: nowrap;
}

@media (max-width: 60rem) {
	.email-box,
	.webhook-box {
		max-width: unset;
	}
}
</style>
