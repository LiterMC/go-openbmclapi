<script setup lang="ts">
import { inject, type Ref } from 'vue'
import { useRouter } from 'vue-router'
import LoginComp from '@/components/LoginComp.vue'
import { tr } from '@/lang'

const props = defineProps<{
	next?: string
}>()

const router = useRouter()
const token = inject('token') as Ref<string | null>

function logged(tk: string): void {
	token.value = tk
	if (!props.next) {
		router.replace('/')
		return
	}
	if (props.next.startsWith('https://') || props.next.startsWith('https://')) {
		window.location.replace(props.next)
		return
	}
	router.replace(props.next)
}
</script>
<template>
	<div class="login-box">
		<h1>{{ tr('title.login') }}</h1>
		<LoginComp @logged="logged" />
	</div>
</template>
<style scoped>
.login-box {
	display: flex;
	flex-direction: column;
	align-items: center;
}
</style>
