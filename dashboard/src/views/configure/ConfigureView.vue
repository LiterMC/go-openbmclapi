<script setup lang="ts">
import { reactive, ref, computed, watch, inject, onMounted, type Ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useRequest } from 'vue-request'
import Accordion from 'primevue/accordion'
import AccordionContent from 'primevue/accordioncontent'
import AccordionHeader from 'primevue/accordionheader'
import AccordionPanel from 'primevue/accordionpanel'
import Button from 'primevue/button'
import Calendar from 'primevue/calendar'
import Card from 'primevue/card'
import FloatLabel from 'primevue/floatlabel'
import InputGroup from 'primevue/inputgroup'
import InputGroupAddon from 'primevue/inputgroupaddon'
import InputIcon from 'primevue/inputicon'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Listbox from 'primevue/listbox'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Password from 'primevue/password'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useToast } from 'primevue/usetoast'
import { getConfig, putConfig, type Config } from '@/api/v0'
import { tr } from '@/lang'

const toast = useToast()
const token = inject('token') as Ref<string | null>

const loading = ref(false)
const saving = ref(false)

const config = reactive<Config>({
	public_host: '',
	public_port: 443,
	host: '0.0.0.0',
	port: 4000,
	use_cert: false,
	allow_unsecure_connection: false,
	trusted_x_forwarded_for: false,

	only_gc_when_start: false,
	sync_interval: 10,
	download_max_conn: 16,
	max_reconnect_count: 10,

	log_slots: 7,
	no_access_log: false,
	access_log_slots: 16,

	clusters: {
		'test-cluster': {
			id: '11123344',
			secret: 'a-secret',
			byoc: true,
			public_hosts: ['localhost', 'some.example.com'],
			server: 'https://openbmclapi.bangbang93.com',
			skip_signature_check: false,
			storages: ['test-local-storage', 'test-webdav-storage'],
		},
	},

	storages: [
		{
			id: 'test-local-storage',
			weight: 100,
			type: 'local',
			cache_path: 'cache',
			compressor: '',
		},
		{
			id: 'test-webdav-storage',
			weight: 100,
			type: 'webdav',
			max_conn: 10,
			max_upload_rate: 1024,
			max_download_rate: 4096,
			pre_gen_measures: true,
			follow_redirect: false,
			redirect_link_cache: 10,
		},
	],

	certificates: [],

	tunneler: {
		enable: false,
		tunnel_program: './path/to/tunnel/program',
		output_regex: '\\bNATedAddr\\s+(?P<host>[0-9.]+|\\[[0-9a-f:]+\\]):(?P<port>\\d+)$',
	},

	cache: {
		type: 'memory',
	},

	serve_limit: {
		enable: false,
		max_conn: 16384,
		upload_rate: 1024 * 12,
	},

	api_rate_limit: {
		anonymous: {
			per_minute: 10,
			per_hour: 120,
		},
		logged: {
			per_minute: 120,
			per_hour: 6000,
		},
	},

	notification: {
		enable_email: false,
		email_smtp: 'smtp.example.com:25',
		email_smtp_encryption: 'tls',
		email_sender: 'noreply@example.com',
		email_sender_password: 'example-password',
		enable_webhook: true,
	},

	dashboard: {
		enable: true,
		username: '',
		password: '',
		pwa_name: 'GoOpenBmclApi Dashboard',
		pwa_short_name: 'GOBA Dash',
		pwa_description: 'Go-Openbmclapi Internal Dashboard',
		notification_subject: 'mailto:user@example.com',
	},

	github_api: {
		update_check_interval: 60 * 60 * 1e9,
		authorization: '',
	},

	database: {
		driver: 'sqlite',
		data_source_name: 'data/files.db',
	},

	hijack: {
		enable: false,
		enable_local_cache: false,
		local_cache_path: 'hijack_cache',
		require_auth: false,
		auth_users: [
			{
				username: 'example-username',
				password: 'example-password',
			},
		],
	},

	webdav_users: {
		//
	},

	advanced: {}, // unused
})

const changingConfig = reactive<Config>(JSON.parse(JSON.stringify(config)))
const configChanged = computed(() => JSON.stringify(changingConfig) !== JSON.stringify(config)) // TODO: use deep equal

async function refreshConfig(): Promise<void> {
	if (!token.value) {
		toast.add({
			severity: 'error',
			summary: tr('message.settings.login.auth'),
			detail: tr('message.settings.login.first'),
			life: 5000,
		})
		return
	}
	loading.value = true
	try {
		const configValue = await getConfig(token.value)
		Object.assign(config, configValue)
		Object.assign(changingConfig, JSON.parse(JSON.stringify(configValue)))
	} finally {
		loading.value = false
	}
}

async function onSaveConfig(): Promise<void> {
	if (!token.value) {
		toast.add({
			severity: 'error',
			summary: tr('message.settings.login.auth'),
			detail: tr('message.settings.login.first'),
			life: 5000,
		})
		return
	}
	saving.value = true
	try {
		await putConfig(token.value, changingConfig)
	} finally {
		saving.value = false
	}
}

onMounted(() => {
	refreshConfig()
})
</script>
<template>
	<div>
		<div class="header">
			<h1 class="flex-row-center">
				<i class="pi pi-wrench" style="font-size: 0.85em"></i>
				<span style="margin: 0 0.8rem">{{ tr('title.configure') }}</span>
			</h1>
			<div class="flex-row-center">
				<Button
					icon="pi pi-save"
					:label="tr(configChanged ? 'button.save' : 'button.saved')"
					severity="success"
					rounded
					:disabled="saving || loading || !configChanged"
					:loading="saving"
					@click="onSaveConfig"
				/>
				<Button
					style="margin: 0 0.8rem"
					icon="pi pi-sync"
					:label="tr('button.refresh')"
					severity="secondary"
					rounded
					:disabled="saving || loading"
					:loading="loading"
					@click="refreshConfig"
				/>
			</div>
		</div>

		<div v-if="!token">
			<Message severity="error">
				{{ tr('message.settings.login.first') }}
			</Message>
		</div>
		<div v-else-if="loading">
			<Message>Loading ...</Message>
		</div>
		<template v-else-if="config && changingConfig">
			<Card class="configure-group">
				<template #title>
					<div class="flex-row-center configure-group-title">
						<label>{{ tr('title.configures.hostport') }}</label>
					</div>
				</template>
				<template #content>
					<div class="configure-elem">
						<FloatLabel variant="on">
							<InputText type="text" v-model="changingConfig.public_host" />
							<label>{{ tr('title.configures.item.public_host') }}</label>
						</FloatLabel>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.public_host') }}
						</Message>
					</div>
					<div class="configure-elem">
						<FloatLabel variant="on">
							<InputNumber
								v-model="changingConfig.public_port"
								:useGrouping="false"
								:min="1"
								:max="65535"
								showButtons
							/>
							<label>{{ tr('title.configures.item.public_port') }}</label>
						</FloatLabel>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.public_port') }}
						</Message>
					</div>
					<div class="configure-elem">
						<FloatLabel variant="on">
							<InputText type="text" v-model="changingConfig.host" />
							<label>{{ tr('title.configures.item.host') }}</label>
						</FloatLabel>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.host') }}
						</Message>
					</div>
					<div class="configure-elem">
						<FloatLabel variant="on">
							<InputNumber
								v-model="changingConfig.port"
								:useGrouping="false"
								:min="1"
								:max="65535"
								showButtons
							/>
							<label>{{ tr('title.configures.item.port') }}</label>
						</FloatLabel>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.port') }}
						</Message>
					</div>
					<div class="configure-elem">
						<div class="configure-button-elem">
							<label>{{ tr('title.configures.item.use_cert') }}</label>
							<ToggleSwitch v-model="changingConfig.use_cert" />
						</div>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.use_cert') }}
						</Message>
					</div>
					<div class="configure-elem">
						<div class="configure-button-elem">
							<label>{{ tr('title.configures.item.trusted_x_forwarded_for') }}</label>
							<ToggleSwitch v-model="changingConfig.trusted_x_forwarded_for" />
						</div>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.trusted_x_forwarded_for') }}
						</Message>
					</div>
				</template>
			</Card>

			<Card class="configure-group">
				<template #title>
					<div class="flex-row-center configure-group-title">
						<label>{{ tr('title.configures.sync') }}</label>
					</div>
				</template>
				<template #content>
					<div class="configure-elem">
						<div class="configure-button-elem">
							<label>{{ tr('title.configures.item.only_gc_when_start') }}</label>
							<ToggleSwitch v-model="changingConfig.only_gc_when_start" />
						</div>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.only_gc_when_start') }}
						</Message>
					</div>
					<div class="configure-elem">
						<FloatLabel variant="on">
							<InputNumber
								v-model="changingConfig.sync_interval"
								:useGrouping="false"
								:min="0"
								suffix="m"
								showButtons
							/>
							<label>{{ tr('title.configures.item.sync_interval') }}</label>
						</FloatLabel>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.sync_interval') }}
						</Message>
					</div>
					<div class="configure-elem">
						<FloatLabel variant="on">
							<InputNumber
								v-model="changingConfig.download_max_conn"
								:useGrouping="false"
								:min="0"
								showButtons
							/>
							<label>{{ tr('title.configures.item.download_max_conn') }}</label>
						</FloatLabel>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.download_max_conn') }}
						</Message>
					</div>
					<div class="configure-elem">
						<FloatLabel variant="on">
							<InputNumber
								v-model="changingConfig.max_reconnect_count"
								:useGrouping="false"
								:min="0"
								showButtons
							/>
							<label>{{ tr('title.configures.item.max_reconnect_count') }}</label>
						</FloatLabel>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.max_reconnect_count') }}
						</Message>
					</div>
				</template>
			</Card>

			<Card class="configure-group">
				<template #title>
					<div class="flex-row-center configure-group-title">
						<label>{{ tr('title.configures.logs') }}</label>
					</div>
				</template>
				<template #content>
					<div class="configure-elem">
						<FloatLabel variant="on">
							<InputNumber
								v-model="changingConfig.log_slots"
								:useGrouping="false"
								:min="0"
								showButtons
							/>
							<label>{{ tr('title.configures.item.log_slots') }}</label>
						</FloatLabel>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.log_slots') }}
						</Message>
					</div>
					<div class="configure-elem">
						<div class="configure-button-elem">
							<label>{{ tr('title.configures.item.enable_access_log') }}</label>
							<ToggleSwitch
								:modelValue="!changingConfig.no_access_log"
								@update:modelValue="(v) => changingConfig && (changingConfig.no_access_log = !v)"
							/>
						</div>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.enable_access_log') }}
						</Message>
					</div>
					<div class="configure-elem">
						<FloatLabel variant="on">
							<InputNumber
								v-model="changingConfig.access_log_slots"
								:useGrouping="false"
								:min="0"
								showButtons
							/>
							<label>{{ tr('title.configures.item.access_log_slots') }}</label>
						</FloatLabel>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.access_log_slots') }}
						</Message>
					</div>
				</template>
			</Card>

			<Card class="configure-group">
				<template #title>
					<div class="flex-row-center configure-group-title">
						<label>{{ tr('title.configures.storages') }}</label>
					</div>
				</template>
				<template #content>
					<Accordion>
						<AccordionPanel
							v-for="storage in changingConfig.storages"
							:key="storage.id"
							:value="storage.id"
						>
							<AccordionHeader>{{ storage.id }}</AccordionHeader>
							<AccordionContent>
								<div class="configure-elem">
									<FloatLabel variant="on">
										<Select v-model="storage.type" :options="['local', 'mount', 'webdav']" />
										<label>{{ tr('title.configures.item.storage.type') }}</label>
									</FloatLabel>
									<Message size="small" severity="secondary" variant="simple">
										{{ tr('description.configures.item.storage.type') }}
									</Message>
								</div>
								<div class="configure-elem">
									<FloatLabel variant="on">
										<InputNumber
											type="text"
											v-model="storage.weight"
											:useGrouping="false"
											:min="0"
											showButtons
										/>
										<label>{{ tr('title.configures.item.storage.weight') }}</label>
									</FloatLabel>
									<Message size="small" severity="secondary" variant="simple">
										{{ tr('description.configures.item.storage.weight') }}
									</Message>
								</div>
								<template v-if="storage.type === 'local'">
									<div class="configure-elem">
										<FloatLabel variant="on">
											<InputText type="text" v-model="storage.cache_path" />
											<label>{{ tr('title.configures.item.storage.cache_path') }}</label>
										</FloatLabel>
										<Message size="small" severity="secondary" variant="simple">
											{{ tr('description.configures.item.storage.cache_path') }}
										</Message>
									</div>
								</template>
								<template v-if="storage.type === 'mount'">
									<div class="configure-elem">
										<FloatLabel variant="on">
											<InputText type="text" v-model="storage.path" />
											<label>{{ tr('title.configures.item.storage.path') }}</label>
										</FloatLabel>
										<Message size="small" severity="secondary" variant="simple">
											{{ tr('description.configures.item.storage.path') }}
										</Message>
									</div>
									<div class="configure-elem">
										<FloatLabel variant="on">
											<InputText type="text" v-model="storage.redirect_base" />
											<label>{{ tr('title.configures.item.storage.redirect_base') }}</label>
										</FloatLabel>
										<Message size="small" severity="secondary" variant="simple">
											{{ tr('description.configures.item.storage.redirect_base') }}
										</Message>
									</div>
									<div class="configure-elem">
										<div class="configure-button-elem">
											<label>{{ tr('title.configures.item.storage.pre_gen_measures') }}</label>
											<ToggleSwitch v-model="storage.pre_gen_measures" />
										</div>
										<Message size="small" severity="secondary" variant="simple">
											{{ tr('description.configures.item.storage.pre_gen_measures') }}
										</Message>
									</div>
								</template>
								<template v-if="storage.type === 'webdav'">
									<div class="configure-elem">
										<FloatLabel variant="on">
											<InputNumber
												v-model="storage.max_conn"
												:useGrouping="false"
												:min="0"
												showButtons
											/>
											<label>{{ tr('title.configures.item.storage.max_conn') }}</label>
										</FloatLabel>
										<Message size="small" severity="secondary" variant="simple">
											{{ tr('description.configures.item.storage.max_conn') }}
										</Message>
									</div>
									<div class="configure-elem">
										<FloatLabel variant="on">
											<InputNumber
												v-model="storage.max_upload_rate"
												:useGrouping="false"
												:min="0"
												suffix="KiB/s"
												showButtons
											/>
											<label>{{ tr('title.configures.item.storage.max_upload_rate') }}</label>
										</FloatLabel>
										<Message size="small" severity="secondary" variant="simple">
											{{ tr('description.configures.item.storage.max_upload_rate') }}
										</Message>
									</div>
									<div class="configure-elem">
										<FloatLabel variant="on">
											<InputNumber
												v-model="storage.max_download_rate"
												:useGrouping="false"
												:min="0"
												suffix="KiB/s"
												showButtons
											/>
											<label>{{ tr('title.configures.item.storage.max_download_rate') }}</label>
										</FloatLabel>
										<Message size="small" severity="secondary" variant="simple">
											{{ tr('description.configures.item.storage.max_download_rate') }}
										</Message>
									</div>
									<div class="configure-elem">
										<div class="configure-button-elem">
											<label>{{ tr('title.configures.item.storage.pre_gen_measures') }}</label>
											<ToggleSwitch v-model="storage.pre_gen_measures" />
										</div>
										<Message size="small" severity="secondary" variant="simple">
											{{ tr('description.configures.item.storage.pre_gen_measures') }}
										</Message>
									</div>
									<div class="configure-elem">
										<div class="configure-button-elem">
											<label>{{ tr('title.configures.item.storage.follow_redirect') }}</label>
											<ToggleSwitch v-model="storage.follow_redirect" />
										</div>
										<Message size="small" severity="secondary" variant="simple">
											{{ tr('description.configures.item.storage.follow_redirect') }}
										</Message>
									</div>
									<div class="configure-elem">
										<FloatLabel variant="on">
											<InputNumber
												v-model="storage.redirect_link_cache"
												:useGrouping="false"
												:min="0"
												showButtons
											/>
											<label>{{ tr('title.configures.item.storage.redirect_link_cache') }}</label>
										</FloatLabel>
										<Message size="small" severity="secondary" variant="simple">
											{{ tr('description.configures.item.storage.redirect_link_cache') }}
										</Message>
									</div>
								</template>
							</AccordionContent>
						</AccordionPanel>
					</Accordion>
				</template>
			</Card>

			<Card class="configure-group">
				<template #title>
					<div class="flex-row-center configure-group-title">
						<label>{{ tr('title.configures.tunneler') }}</label>
					</div>
				</template>
				<template #content>
					<div class="configure-elem">
						<div class="configure-button-elem">
							<label>{{ tr('title.configures.item.tunneler.enable') }}</label>
							<ToggleSwitch v-model="changingConfig.tunneler.enable" />
						</div>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.tunneler.enable') }}
						</Message>
					</div>
					<div class="configure-elem">
						<FloatLabel variant="on">
							<InputText type="text" v-model="changingConfig.tunneler.tunnel_program" />
							<label>{{ tr('title.configures.item.tunneler.tunnel_program') }}</label>
						</FloatLabel>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.tunneler.tunnel_program') }}
						</Message>
					</div>
					<div class="configure-elem">
						<FloatLabel variant="on">
							<Textarea
								v-model="changingConfig.tunneler.output_regex"
								rows="1"
								autoResize
								autocomplete="off"
								autocorrect="off"
								autocapitalize="none"
								spellcheck="false"
							/>
							<label>{{ tr('title.configures.item.tunneler.output_regex') }}</label>
						</FloatLabel>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.tunneler.output_regex') }}
						</Message>
					</div>
				</template>
			</Card>

			<Card class="configure-group">
				<template #title>
					<div class="flex-row-center configure-group-title">
						<label>{{ tr('title.configures.cache') }}</label>
					</div>
				</template>
				<template #content>
					<div class="configure-elem">
						<FloatLabel variant="on">
							<Select
								v-model="changingConfig.cache.type"
								:options="['no-cache', 'memory', 'redis']"
							/>
							<label>{{ tr('title.configures.item.cache.type') }}</label>
						</FloatLabel>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.cache.type') }}
						</Message>
					</div>
				</template>
			</Card>

			<Card class="configure-group">
				<template #title>
					<div class="flex-row-center configure-group-title">
						<label>{{ tr('title.configures.serve_limit') }}</label>
					</div>
				</template>
				<template #content>
					<div class="configure-elem">
						<div class="configure-button-elem">
							<label>{{ tr('title.configures.item.serve_limit.enable') }}</label>
							<ToggleSwitch v-model="changingConfig.serve_limit.enable" />
						</div>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.serve_limit.enable') }}
						</Message>
					</div>
					<div class="configure-elem">
						<FloatLabel variant="on">
							<InputNumber
								type="text"
								v-model="changingConfig.serve_limit.max_conn"
								:useGrouping="false"
								:min="-1"
								showButtons
							/>
							<label>{{ tr('title.configures.item.serve_limit.max_conn') }}</label>
						</FloatLabel>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.serve_limit.max_conn') }}
						</Message>
					</div>
					<div class="configure-elem">
						<FloatLabel variant="on">
							<InputNumber
								type="text"
								v-model="changingConfig.serve_limit.upload_rate"
								:useGrouping="false"
								:min="0"
								suffix="KiB/s"
								showButtons
							/>
							<label>{{ tr('title.configures.item.serve_limit.upload_rate') }}</label>
						</FloatLabel>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.serve_limit.upload_rate') }}
						</Message>
					</div>
				</template>
			</Card>
		</template>
	</div>
</template>
<style scoped>

.header {
	display: flex;
	flex-direction: row;
	align-items: center;
}

.configure-group {
	width: 36rem;
	font-size: 1rem;
	margin-bottom: 2rem;
	margin-left: 5rem;
}

.configure-group-title {
	justify-content: space-between;
	width: 100%;
	padding: 0 1rem;
}

.configure-group:deep() .p-accordioncontent-content {
	padding: 0;
}

.configure-elem {
	display: flex;
	flex-direction: column;
	width: 100%;
	padding: 0.6rem 1rem;
	background-color: var(--table-row-bg-a);
}

.configure-elem:nth-child(even) {
	background-color: var(--table-row-bg-b);
}

.configure-elem > .p-message {
	margin-top: 0.2rem;
}

.configure-elem:deep() .p-inputtext,
.configure-elem:deep() .p-inputnumber,
.configure-elem:deep() .p-multiselect {
	width: 21rem;
}

.configure-elem:deep() .p-textarea {
	width: 100%;
}

.configure-button-elem {
	display: flex;
	flex-direction: row;
	align-items: center;
	justify-content: space-between;
}

.configure-button-elem > label {
	max-width: calc(100% - 3rem);
}

@media (max-width: 60rem) {
	.header {
		flex-direction: column;
		align-items: flex-start;
	}

	.header > h1 {
		margin-bottom: 0.5rem;
	}

	.header {
		margin-bottom: 1rem;
	}

	.configure-group {
		width: 100%;
		margin-left: 0;
	}

	.configure-group:deep() > .p-card-body {
		padding-left: 0;
		padding-right: 0;
	}

	.configure-elem:deep() .p-inputtext,
	.configure-elem:deep() .p-inputnumber {
		width: 100%;
	}
}
</style>
