<script setup lang="ts">
import { ref, computed, inject, onMounted, type Ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useRequest } from 'vue-request'
import Accordion from 'primevue/accordion'
import AccordionContent from 'primevue/accordioncontent'
import AccordionHeader from 'primevue/accordionheader'
import AccordionPanel from 'primevue/accordionpanel'
import Button from 'primevue/button'
import Calendar from 'primevue/calendar'
import Card from 'primevue/card'
import Dropdown from 'primevue/dropdown'
import FloatLabel from 'primevue/floatlabel'
import InputGroup from 'primevue/inputgroup'
import InputGroupAddon from 'primevue/inputgroupaddon'
import InputIcon from 'primevue/inputicon'
import InputNumber from 'primevue/inputnumber'
import InputSwitch from 'primevue/inputswitch'
import InputText from 'primevue/inputtext'
import Listbox from 'primevue/listbox'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Password from 'primevue/password'
import Select from 'primevue/select'
import { useToast } from 'primevue/usetoast'
import { getConfig, type Config } from '@/api/v0'
import { tr } from '@/lang'

const toast = useToast()
const token = inject('token') as Ref<string | null>

const loading = ref(false)
const config = ref<Config | null>(null)
const changingConfig = ref<Config | null>(null)

changingConfig.value = config.value = {
	clusters: {
		'test-cluster': {
			id: '11123344',
			secret: 'a-secret',
			byoc: true,
			public_hosts: ['localhost', 'some.example.com'],
		},
	},
}

const configChanged = computed(
	() => JSON.stringify(changingConfig.value) !== JSON.stringify(config.value),
)

async function refreshConfig() {
	if (!token.value) {
		return
	}
	loading.value = true
	try {
		changingConfig.value = config.value = await getConfig(token.value)
	} finally {
		loading.value = false
	}
}

onMounted(() => {
	refreshConfig()
})
</script>
<template>
	<div>
		<h1>
			<i class="pi pi-wrench" style="font-size: 0.85em"></i>
			{{ tr('title.configure') }}
		</h1>

		<div v-if="loading">
			<i>Loading ...</i>
		</div>
		<template v-else-if="config">
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
							<InputSwitch v-model="changingConfig.use_cert" />
						</div>
						<Message size="small" severity="secondary" variant="simple">
							{{ tr('description.configures.item.use_cert') }}
						</Message>
					</div>
					<div class="configure-elem">
						<div class="configure-button-elem">
							<label>{{ tr('title.configures.item.trusted_x_forwarded_for') }}</label>
							<InputSwitch v-model="changingConfig.trusted_x_forwarded_for" />
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
							<InputSwitch v-model="changingConfig.only_gc_when_start" />
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
							<InputSwitch v-model="changingConfig.enable_access_log" />
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
						<label>{{ tr('title.configures.clusters') }}</label>
					</div>
				</template>
				<template #content>
					<Accordion>
						<AccordionPanel
							v-for="(cluster, name) in changingConfig.clusters"
							:key="name"
							:value="name"
						>
							<AccordionHeader>{{ name }}</AccordionHeader>
							<AccordionContent>
								<div class="configure-elem">
									<FloatLabel variant="on">
										<InputText type="text" v-model="cluster.id" />
										<label>{{ tr('title.configures.item.cluster.id') }}</label>
									</FloatLabel>
									<Message size="small" severity="secondary" variant="simple">
										{{ tr('description.configures.item.cluster.id') }}
									</Message>
								</div>
								<div class="configure-elem">
									<FloatLabel variant="on">
										<Password v-model="cluster.secret" :feedback="false" />
										<label>{{ tr('title.configures.item.cluster.secret') }}</label>
									</FloatLabel>
									<Message size="small" severity="secondary" variant="simple">
										{{ tr('description.configures.item.cluster.secret') }}
									</Message>
								</div>
								<div class="configure-elem">
									<div class="configure-button-elem">
										<label>{{ tr('title.configures.item.cluster.byoc') }}</label>
										<InputSwitch v-model="cluster.byoc" />
									</div>
									<Message size="small" severity="secondary" variant="simple">
										{{ tr('description.configures.item.cluster.byoc') }}
									</Message>
								</div>
								<div class="configure-elem">
									<Listbox :options="cluster.public_hosts" :highlightOnSelect="false">
										<template #header>
											<InputGroup>
												<FloatLabel variant="on">
													<InputText type="text" style="width: 100%" />
													<label>{{ tr('title.configures.item.cluster.public_hosts') }}</label>
												</FloatLabel>
												<InputGroupAddon>
													<Button icon="pi pi-plus" severity="success" />
												</InputGroupAddon>
											</InputGroup>
										</template>
										<template #option="{ option }">
											<div
												class="flex-row-center"
												style="width: 100%; justify-content: space-between"
											>
												<div>{{ option }}</div>
												<Button icon="pi pi-minus" severity="danger" rounded />
											</div>
										</template>
									</Listbox>
									<Message size="small" severity="secondary" variant="simple">
										{{ tr('description.configures.item.cluster.public_hosts') }}
									</Message>
								</div>
								<div class="configure-elem">
									<FloatLabel variant="on">
										<InputText type="text" v-model="cluster.server" />
										<label>{{ tr('title.configures.item.cluster.server') }}</label>
									</FloatLabel>
									<Message size="small" severity="secondary" variant="simple">
										{{ tr('description.configures.item.cluster.server') }}
									</Message>
								</div>
								<div class="configure-elem">
									<div class="configure-button-elem">
										<label>{{ tr('title.configures.item.cluster.skip_signature_check') }}</label>
										<InputSwitch v-model="cluster.skip_signature_check" />
									</div>
									<Message size="small" severity="secondary" variant="simple">
										{{ tr('description.configures.item.cluster.skip_signature_check') }}
									</Message>
								</div>
								<div class="configure-elem">
									<FloatLabel variant="on">
										<MultiSelect
											v-model="cluster.storages"
											:options="changingConfig.storages"
											optionLabel="id"
											optionValue="id"
										/>
										<label>{{ tr('title.configures.item.cluster.storages') }}</label>
									</FloatLabel>
									<Message size="small" severity="secondary" variant="simple">
										{{ tr('description.configures.item.cluster.storages') }}
									</Message>
								</div>
							</AccordionContent>
						</AccordionPanel>
					</Accordion>
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
										<label>{{ tr('title.configures.item.cluster.weight') }}</label>
									</FloatLabel>
									<Message size="small" severity="secondary" variant="simple">
										{{ tr('description.configures.item.cluster.weight') }}
									</Message>
								</div>
							</AccordionContent>
						</AccordionPanel>
					</Accordion>
				</template>
			</Card>
		</template>
	</div>
</template>
<style scoped>
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

.configure-elem {
	display: flex;
	flex-direction: column;
	width: 100%;
	padding: 0.6rem 1rem;
	background-color: var(--p-surface-100);
}

.configure-elem:nth-child(even) {
	background-color: var(--p-surface-200);
}

.configure-elem > .p-message {
	margin-top: 0.2rem;
}

.configure-elem:deep() .p-inputtext,
.configure-elem:deep() .p-inputnumber,
.configure-elem:deep() .p-multiselect {
	width: 21rem;
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
