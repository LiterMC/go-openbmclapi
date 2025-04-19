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
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import { type ClusterOptions } from '@/api/v0'
import { tr } from '@/lang'

const confirm = useConfirm()
const toast = useToast()
const token = inject('token') as Ref<string | null>

const loading = ref(false)

const clusters = reactive<{ [name: string]: ClusterOptions }>({
	'test-cluster': {
		id: '11123344',
		secret: 'a-secret',
		byoc: true,
		public_hosts: ['localhost', 'some.example.com'],
		server: 'https://openbmclapi.bangbang93.com',
		skip_signature_check: false,
		storages: ['test-local-storage', 'test-webdav-storage'],
	},
	'test-cluster-2': {
		id: '11123344',
		secret: 'a-secret',
		byoc: true,
		public_hosts: ['localhost', 'some.example.com'],
		server: 'https://openbmclapi.bangbang93.com',
		skip_signature_check: false,
		storages: ['test-local-storage', 'test-webdav-storage'],
	},
})
const changingClusters = reactive<{ [name: string]: ClusterOptions }>(JSON.parse(JSON.stringify(clusters)))
const avaliableStorages = reactive<string[]>(['test-local-storage', 'test-webdav-storage'])

const savingFlags = reactive<{ [name: string]: true }>({})
const savingClusters = computed(() => {
	const savingClusters = {}
	for (const name in changingClusters) {
		savingClusters[name] = savingFlags[name] ? 2 : JSON.stringify(changingClusters[name]) !== JSON.stringify(clusters[name]) ? 1 : 0
	}
	return savingClusters
})
const changingClusterPublicHost = reactive<{ [name: string]: string }>({})

const newClusterNameInput = ref()
const newClusterName = ref('')

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
		Object.keys(changingClusterPublicHost).forEach((name) => delete changingClusterPublicHost[name])
	} finally {
		loading.value = false
	}
}

async function onCreateCluster(event: MouseEvent): Promise<void> {
	if (event.target === newClusterNameInput.value.$el) {
		return
	}
	const newName = newClusterName.value.trim()
	if (newName.length === 0) {
		newClusterNameInput.value.$el.focus()
		return
	}
	if (changingClusters[newName]) {
		return
	}
	(changingClusters[newName] as any) = {}
	newClusterName.value = ''
}

function confirmRemoveCluster(clusterName: string): void {
	confirm.require({
		message: tr('message.configures.confirm.remove', clusterName),
		header: tr('title.configures.confirm.remove'),
		icon: 'pi pi-exclamation-triangle',
		acceptProps: {
			label: tr('button.remove'),
			severity: 'danger',
		},
		rejectProps: {
			label: tr('button.cancel'),
			severity: 'secondary',
			outlined: true
		},
		accept: () => onRemoveCluster(clusterName),
	});
}

async function onRemoveCluster(clusterName: string): Promise<void> {
	const cluster = changingClusters[clusterName]
	if (!cluster) {
		return
	}
	delete changingClusters[clusterName]
}

async function onSaveCluster(clusterName: string): Promise<void> {
	const cluster = changingClusters[clusterName]
	if (!cluster) {
		return
	}
	savingFlags[clusterName] = true
	try {
		//
		clusters[clusterName] = JSON.parse(JSON.stringify(cluster))
	} finally {
		delete savingFlags[clusterName]
	}
}

function onCancelClusterChange(clusterName: string): void {
	const cluster = clusters[clusterName]
	if (!cluster) {
		return
	}
	changingClusters[clusterName] = JSON.parse(JSON.stringify(cluster))
}

async function onAddClusterPublicHost(clusterName: string): Promise<void> {
	const cluster = changingClusters[clusterName]
	if (!cluster) {
		return
	}
	let hostname = changingClusterPublicHost[clusterName]
	if (!hostname || !(hostname = hostname.trim())) {
		return
	}
	if (cluster.public_hosts.includes(hostname)) {
		return
	}
	changingClusterPublicHost[clusterName] = ''
	cluster.public_hosts.push(hostname)
}

</script>
<template>
	<div>
		<div class="header">
			<h1 class="flex-row-center">
				<i class="pi pi-server" style="font-size: 0.85em"></i>
				<span style="margin: 0 0.8rem">{{ tr('title.configures.clusters') }}</span>
			</h1>
		</div>
		<div class="body">
			<Accordion>
				<AccordionPanel
					v-for="(cluster, name) in changingClusters"
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
								<ToggleSwitch v-model="cluster.byoc" />
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
											<InputText
												v-model="changingClusterPublicHost[name]"
												type="text"
												style="width: 100%"
											/>
											<label>{{ tr('title.configures.item.cluster.public_hosts') }}</label>
										</FloatLabel>
										<InputGroupAddon>
											<Button
												icon="pi pi-plus"
												severity="success"
												@click="onAddClusterPublicHost(name as string)"
											/>
										</InputGroupAddon>
									</InputGroup>
								</template>
								<template #option="{ index, option }">
									<div
										class="flex-row-center"
										style="width: 100%; justify-content: space-between"
									>
										<div>{{ option }}</div>
										<Button
											icon="pi pi-minus"
											severity="danger"
											rounded
											@click="cluster.public_hosts.splice(index, 1)"
										/>
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
								<ToggleSwitch v-model="cluster.skip_signature_check" />
							</div>
							<Message size="small" severity="secondary" variant="simple">
								{{ tr('description.configures.item.cluster.skip_signature_check') }}
							</Message>
						</div>
						<div class="configure-elem">
							<FloatLabel variant="on">
								<MultiSelect
									v-model="cluster.storages"
									:options="avaliableStorages"
								/>
								<label>{{ tr('title.configures.item.cluster.storages') }}</label>
							</FloatLabel>
							<Message size="small" severity="secondary" variant="simple">
								{{ tr('description.configures.item.cluster.storages') }}
							</Message>
						</div>
						<div class="configure-elem">
							<Button
								fluid
								icon="pi pi-trash"
								:label="tr('button.remove')"
								severity="danger"
								@click="confirmRemoveCluster(name as string)"
							/>
							<div class="flex-row-center" style="margin-top: 1rem">
								<Button
									fluid
									style="margin-right: 1rem"
									icon="pi pi-save"
									:label="tr(savingClusters[name] === 0 ? 'button.saved' : 'button.save')"
									:disabled="savingClusters[name] !== 1"
									:loading="savingClusters[name] === 2"
									severity="success"
									:raised="savingClusters[name] === 1"
									@click="onSaveCluster(name as string)"
								/>
								<Button
									fluid
									icon="pi pi-undo"
									:label="tr('button.undo')"
									:disabled="savingClusters[name] !== 1"
									severity="warn"
									outlined
									:raised="savingClusters[name] === 1"
									@click="onCancelClusterChange(name as string)"
								/>
							</div>
						</div>
					</AccordionContent>
				</AccordionPanel>
				<AccordionPanel value="+">
					<template #default>
						<button class="p-accordionheader" @click="onCreateCluster">
							<InputText
								ref="newClusterNameInput"
								v-model="newClusterName"
								placeholder="Cluster Name"
							/>
							<i class="pi pi-plus"></i>
						</button>
					</template>
				</AccordionPanel>
			</Accordion>
		</div>
	</div>
</template>
<style scoped>

.header {
	display: flex;
	flex-direction: row;
	align-items: center;
}

.body {
	width: 30rem;
	position: relative;
}

.body:deep() .p-accordioncontent-content {
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

	.body {
		width: 100%;
		margin-left: 0;
	}

	.configure-elem:deep() .p-inputtext,
	.configure-elem:deep() .p-inputnumber {
		width: 100%;
	}
}
</style>

