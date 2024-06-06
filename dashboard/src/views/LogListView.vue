<script setup lang="ts">
import { ref, inject, onMounted, type Ref } from 'vue'
import Button from 'primevue/button'
import { useToast } from 'primevue/usetoast'
import FileListCard from '@/components/FileListCard.vue'
import {
	getLogFiles,
	getLogFile,
	getLogFileURL,
	type FileInfo,
} from '@/api/v0'
import { tr } from '@/lang'

const toast = useToast()
const token = inject('token') as Ref<string | null>

const logList = ref<FileInfo[] | null>(null)
async function refreshFileList(): Promise<void> {
	if (!token.value) {
		return
	}
	logList.value = await getLogFiles(token.value)
}

async function openFile(file: FileInfo): Promise<void> {
	if (!token.value) {
		return
	}
	const buf = await getLogFile(token.value, file.name, true)
	console.log(buf)
	alert('TODO: display log file content')
}

async function downloadLogFile(file: FileInfo): Promise<void> {
	if (!token.value) {
		return
	}
	const u = await getLogFileURL(token.value, file.name, false)
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
		<FileListCard class="filelist-card" :name="'logs/'" :files="logList" v-slot="{ index, file }" @click="openFile">
			<Button icon="pi pi-file-export" @click="downloadLogFile(file)"/>
		</FileListCard>
	</div>
</template>
<style scoped>
.filelist-card {
}
</style>
