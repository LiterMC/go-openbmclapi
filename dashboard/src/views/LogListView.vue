<script setup lang="ts">
import { ref, inject, onMounted, type Ref } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import { useToast } from 'primevue/usetoast'
import FileListCard from '@/components/FileListCard.vue'
import FileContentCard from '@/components/FileContentCard.vue'
import { getLogFiles, getLogFile, getLogFileURL, type FileInfo } from '@/api/v0'
import { tr } from '@/lang'

const toast = useToast()
const token = inject('token') as Ref<string | null>

const logList = ref<FileInfo[] | null>(null)
async function refreshFileList(): Promise<void> {
	if (!token.value) {
		toast.add({
			severity: 'warn',
			summary: tr('message.settings.login.auth'),
			detail: tr('message.settings.login.first'),
		})
		return
	}
	var files: FileInfo[]
	try {
		files = await getLogFiles(token.value)
	} catch (e) {
		toast.add({
			severity: 'error',
			summary: tr('message.settings.filelist.cant.fetch'),
			detail: String(e),
		})
		return
	}
	const appLogs: FileInfo[] = []
	const accessLogs: { n: number; f: FileInfo }[] = []
	const otherLogs: FileInfo[] = []
	for (const f of files) {
		if (/^\d{8}-\d{2}\.log(?:\.gz)?$/.test(f.name)) {
			appLogs.push(f)
			continue
		}
		const data = /^access(?:\.(\d+))?\.log(?:\.gz)?$/.exec(f.name)
		if (data) {
			accessLogs.push({ n: data[1] ? Number.parseInt(data[1]) : 0, f: f })
			continue
		}
		otherLogs.push(f)
	}
	appLogs.sort((a, b) => (a < b ? 1 : -1))
	accessLogs.sort(({ n: a }, { n: b }) => a - b)
	const logs: FileInfo[] = []
	logs.push(...appLogs)
	for (const f of accessLogs) {
		logs.push(f.f)
	}
	logs.push(...otherLogs)
	logList.value = logs
}

const showInfo = ref<FileInfo | null>(null)
const fileContent = ref<string | null>(null)

async function openFile(file: FileInfo): Promise<void> {
	if (!token.value) {
		toast.add({
			severity: 'error',
			summary: tr('message.settings.login.auth'),
			detail: tr('message.settings.login.first'),
			life: 10000,
		})
		return
	}
	if (showInfo.value) {
		toast.add({
			severity: 'error',
			summary: 'Another file is opening',
			life: 5000,
		})
		return
	}
	showInfo.value = file
	fileContent.value = null
	var buf: ArrayBuffer
	try {
		buf = await getLogFile(token.value, file.name, true)
	} catch (e) {
		toast.add({
			severity: 'error',
			summary: tr('message.settings.filelist.cant.fetch'),
			detail: String(e),
		})
		return
	}
	if (showInfo.value.name !== file.name) {
		return
	}
	fileContent.value = new TextDecoder().decode(new Uint8Array(buf))
}

async function downloadLogFile(file: FileInfo, noEncrypt?: boolean): Promise<void> {
	if (!token.value) {
		return
	}
	const u = await getLogFileURL(token.value, file.name, noEncrypt)
	window.open(u)
}

onMounted(() => {
	refreshFileList()
})
</script>
<template>
	<div>
		<h1>
			{{ tr('title.logs') }}
		</h1>
		<FileListCard
			class="filelist-card"
			:name="'logs/'"
			:files="logList"
			v-slot="{ index, file }"
			@click="openFile"
		>
			<Button icon="pi pi-file-export" aria-label="Export log" @click="downloadLogFile(file)" />
		</FileListCard>
		<Dialog
			:visible="!!showInfo"
			@update:visible="(show) => !show && ((showInfo = null), (fileContent = null))"
			modal
			:header="(showInfo && showInfo.name) || undefined"
			:style="{ width: 'var(--dialog-width)' }"
		>
			<FileContentCard v-if="showInfo" :info="showInfo" :content="fileContent" v-slot="{ info }">
				<div class="flex-row-center tool-box">
					<Button
						icon="pi pi-download"
						aria-label="Download"
						link
						@click="downloadLogFile(info, true)"
					/>
					<Button icon="pi pi-file-export" label="Export" @click="downloadLogFile(info)" />
				</div>
			</FileContentCard>
		</Dialog>
	</div>
</template>
<style scoped>
.tool-box {
	width: 100%;
	margin-top: 0.3rem;
}

.tool-box > * {
	margin-left: 0.3rem;
}
</style>
