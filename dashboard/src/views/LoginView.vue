<script setup lang="ts">
import { inject, type Ref } from 'vue'
import LoginComp from '@/components/LoginComp.vue'
import { tr } from '@/lang'

const props = defineProps<{
	next?: string
}>()

const token = inject('token') as Ref<string | null>

function logged(tk: string): void {
	token.value = tk
	window.location.replace(props.next || '/')
}

</script>
<template>
	<div class="login-box">
		<h1>{{tr('title.login')}}</h1>
		<LoginComp @logged="logged"/>
	</div>
</template>
<style scoped>

.login-box {
	display: flex;
	flex-direction: column;
	align-items: center;
}

</style>