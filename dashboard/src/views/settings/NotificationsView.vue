<script setup lang="ts">
import { ref, computed, inject, nextTick, onBeforeMount, type Ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import MultiSelect from 'primevue/multiselect'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Skeleton from 'primevue/skeleton'
import { useToast } from 'primevue/usetoast'
import TransitionExpandGroup from '@/components/TransitionExpandGroup.vue'
import {
	ALL_SUBSCRIBE_SCOPES,
	getEmailSubscriptions,
	addEmailSubscription,
	updateEmailSubscription,
	removeEmailSubscription,
	getWebhooks,
	addWebhook,
	updateWebhook,
	removeWebhook,
	type SubscribeScope,
	type EmailItemPayload,
	type EmailItemRes,
	type WebhookItemPayload,
	type WebhookItemRes,
} from '@/api/v0'
import { tr } from '@/lang'

const router = useRouter()
const toast = useToast()
const token = inject('token') as Ref<string | null>

const emails = ref<EmailItemRes[] | null>(null)
const newEmailItem = ref<EmailItemPayload>({
	addr: '',
	scopes: [],
	enabled: true,
})
const newEmailItemInvalid = ref<string | null>(null)
const newEmailItemSaving = ref(false)

const webhookEditingItem = ref<(WebhookItemPayload & { _?: WebhookItemRes }) | null>(null)
const webhookEdited = computed((): boolean => {
	const item = webhookEditingItem.value
	if (!item) {
		return true
	}
	const ori = item._
	if (!ori) {
		return true
	}
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

const webhooks = ref<WebhookItemRes[] | null>()

function toastLoginFirst(): void {
	toast.add({
		severity: 'error',
		summary: tr('message.settings.login.first'),
		life: 5000,
	})
}

function checkEmail(email: string): string | null {
	if (!email.match(/^[\w-\.]+@([\w-]+\.)+[\w-]{2,}$/)) {
		return 'Email is invalid'
	}
	return null
}

async function addEmail(item: EmailItemPayload): Promise<void> {
	if (!token.value) {
		toastLoginFirst()
		return
	}
	newEmailItemSaving.value = true
	try {
		if ((newEmailItemInvalid.value = checkEmail(item.addr))) {
			toast.add({
				severity: 'error',
				summary: 'Email Address is invalid',
				detail: newEmailItemInvalid.value,
				life: 3000,
			})
			return
		}
		if (item.scopes.length === 0) {
			toast.add({
				severity: 'error',
				summary: 'Scopes are invalid',
				detail: 'Please select at least one scope',
				life: 3000,
			})
			return
		}
		await addEmailSubscription(token.value, item)
	} finally {
		newEmailItemSaving.value = false
	}
}

async function removeEmail(index: number): Promise<void> {
	if (!emails.value) {
		return
	}
	if (!token.value) {
		toastLoginFirst()
		return
	}
	const { addr } = emails.value[index]
	try {
		await removeEmailSubscription(token.value, addr)
		emails.value.splice(index, 1)
	} catch (err) {
		toast.add({
			severity: 'error',
			summary: 'Error when removing email subscription',
			detail: String(err),
			life: 3000,
		})
	}
}

function openCreateWebhookDialog(): void {
	webhookEditingItem.value = {
		name: '',
		endpoint: '',
		auth: '',
		scopes: [],
		enabled: true,
	}
}

async function openRemoveWebhookDialog(index: number): Promise<void> {
	if (!token.value) {
		toastLoginFirst()
		return
	}
	if (!webhooks.value) {
		return
	}
	const { id } = webhooks.value[index]
	try {
		await removeWebhook(token.value, id)
		webhooks.value.splice(index, 1)
	} catch (err) {
		toast.add({
			severity: 'error',
			summary: 'Error when removing email subscription',
			detail: String(err),
			life: 3000,
		})
	}
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
	if (name.length === 0) {
		return 'Name cannot be empty'
	}
	if (name.length >= 32) {
		return 'Name must be less than 32 characters'
	}
	return null
}

function checkEndPoint(url: string): string | null {
	if (!url) {
		return 'EndPoint cannot be empty'
	}
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
	if (!token.value) {
		toastLoginFirst()
		return
	}
	webhookEditNameInvalid.value = null
	webhookEditEndpointInvalid.value = null
	webhookEditAuthInvalid.value = null
	webhookEditSaving.value = true
	try {
		const { name, endpoint, auth, scopes, enabled } = webhookEditingItem.value
		const id = webhookEditingItem.value._?.id
		if ((webhookEditNameInvalid.value = checkName(name))) {
			return
		}
		if ((webhookEditEndpointInvalid.value = checkEndPoint(endpoint))) {
			return
		}
		if ((webhookEditAuthInvalid.value = checkAuth(auth))) {
			return
		}
		if (id !== undefined) {
			await updateWebhook(token.value, id, {
				name: name,
				endpoint: endpoint,
				auth: auth === '-' ? undefined : auth,
				scopes: scopes,
				enabled: enabled,
			})
		} else {
			await addWebhook(token.value, {
				name: name,
				endpoint: endpoint,
				auth: auth,
				scopes: scopes,
				enabled: enabled,
			})
		}
		webhookEditingItem.value = null
	} catch (err) {
		toast.add({
			severity: 'error',
			summary: 'Error when saving webhook config',
			detail: String(err),
			life: 3000,
		})
	} finally {
		webhookEditSaving.value = false
	}
}

onBeforeMount(() => {
	if (!token.value) {
		router.replace('/login?next=/settings/notifications')
		return
	}
	getEmailSubscriptions(token.value).then((res) => {
		emails.value = res
	})
	getWebhooks(token.value).then((res) => {
		webhooks.value = res
	})
})
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
		<Card v-if="emails" class="email-box">
			<template #content>
				<DataTable :value="emails" scrollable scrollHeight="min(30rem, 70vh)">
					<Column :header="tr('title.email.recver')" field="addr">
						<template #footer>
							<InputText
								type="email"
								autocomplete="email"
								placeholder="youremail@example.com"
								v-model="newEmailItem.addr"
								:invalid="newEmailItem.addr !== '' && newEmailItemInvalid !== null"
								@input="newEmailItemInvalid = checkEmail(newEmailItem.addr)"
								style="width: 16rem; height: 2.6rem"
							/>
						</template>
					</Column>
					<Column
						:header="tr('title.webhook.scopes')"
						:field="({ scopes }) => scopes.join(', ')"
						style="flex-shrink: 0; width: 10rem"
					>
						<template #footer>
							<MultiSelect
								class="flex-auto width-full"
								:options="ALL_SUBSCRIBE_SCOPES"
								v-model="newEmailItem.scopes"
								placeholder="Select Scopes"
								:invalid="newEmailItem.addr !== '' && newEmailItem.scopes.length === 0"
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
								:loading="newEmailItemSaving"
								@click="addEmail(newEmailItem)"
							/>
						</template>
					</Column>
				</DataTable>
			</template>
		</Card>
		<Skeleton v-else class="email-box" />
	</div>
	<div class="section">
		<div class="flex-row-center section-title">
			<h2>{{ tr('title.webhooks') }}</h2>
			<Button
				class="title-button"
				label="Create New"
				icon="pi pi-plus"
				outlined
				@click="openCreateWebhookDialog"
			/>
		</div>
		<div v-if="webhooks" class="webhook-box">
			<Card v-for="(item, i) in webhooks" :key="item.id" class="margin-1">
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
							@click="openRemoveWebhookDialog(i)"
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
						<i v-else class="flex-auto">No authorization</i>
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
		<Skeleton v-else class="webhook-box" />
		<Dialog
			:visible="webhookEditingItem !== null"
			@update:visible="webhookEditingItem = null"
			modal
			style="width: 40rem"
		>
			<template #header>
				<div>
					<h3 class="font-bold margin-none">
						{{ tr(webhookEditingItem?._ ? 'title.edit' : 'title.create-new') }}
						{{ tr('title.webhooks') }}
					</h3>
				</div>
			</template>
			<template v-if="webhookEditingItem">
				<div v-if="webhookEditingItem._" class="p-text-secondary margin-1">
					{{ tr('title.webhook.configure') }} "{{ webhookEditingItem._.name }}"
				</div>
				<div class="input-box">
					<div class="flex-row-center">
						<label class="webhook-edit-label">{{ tr('title.webhook.name') }}</label>
						<InputText
							class="flex-auto"
							autocomplete="off"
							placeholder="Webhook Name"
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
							placeholder="https://example.com/path/to/endpoint"
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
							placeholder="Authorization"
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
						placeholder="Select Scopes"
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

.webhook-box {
	padding: 1rem 0;
	border-top: 1px var(--surface-border) solid;
	border-bottom: 1px var(--surface-border) solid;
}

.email-box,
.webhook-box {
	max-width: 52rem;
	min-height: 4rem;
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
