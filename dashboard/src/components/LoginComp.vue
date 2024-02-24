<script setup lang="ts">
import { inject, ref, nextTick, type Ref } from 'vue'
import axios from 'axios'
import { sha256 } from 'js-sha256'
import Message from 'primevue/message'
import FloatLabel from 'primevue/floatlabel'
import InputGroup from 'primevue/inputgroup'
import InputGroupAddon from 'primevue/inputgroupaddon'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Button from 'primevue/button'
import { tr } from '@/lang'

const emit = defineEmits<{
	logged: [token: string]
}>()

const username = ref('')
const password = ref('')
const loading = ref(false)
const errMsg = ref<(() => string) | string | null>(null)

async function login(): Promise<void> {
	errMsg.value = null
	await nextTick()
	const user = username.value
	const passwd = sha256(password.value)
	if (!user) {
		errMsg.value = () => tr('message.login.input.username')
		return
	}
	if (!password.value) {
		errMsg.value = () => tr('message.login.input.password')
		return
	}
	loading.value = true
	const res = await axios
		.post(`/api/v0/login`, {
			username: user,
			password: passwd
		})
		.catch((err) => {
			const data = err.response.data
			console.error('LoginError:', err)
			if (data && data.error) {
				errMsg.value = 'LoginError: ' + data.error
				if (data.message) {
					errMsg.value += ': ' + data.message
				}
			} else {
				errMsg.value = err.toString()
			}
			return null
		})
	loading.value = false
	if (!res) {
		return
	}
	const { token: tk } = res.data as { token: string }
	emit('logged', tk)
}
</script>

<template>
	<div class="login">
		<form v-focustrap @submit.prevent="login">
			<InputGroup>
				<InputGroupAddon>
					<i class="pi pi-user"></i>
				</InputGroupAddon>
				<FloatLabel>
					<InputText
						name="username"
						autocomplete="username"
						v-model="username"
					/>
					<label for="username">{{tr('title.username')}}</label>
				</FloatLabel>
			</InputGroup>
			<InputGroup>
				<InputGroupAddon>
					<i class="pi pi-lock"></i>
				</InputGroupAddon>
				<FloatLabel>
					<InputText
						type="password"
						name="password"
						autocomplete="username"
						v-model="password"
					/>
					<label for="password">{{tr('title.password')}}</label>
				</FloatLabel>
			</InputGroup>
			<Button class="login-btn" type="submit" :label="tr('title.login')" :loading="loading" icon="pi pi-sign-in"/>
		</form>
		<Message v-if="errMsg" :closable="false" severity="error">
			{{typeof errMsg === 'function' ? errMsg() : errMsg}}
		</Message>
	</div>
</template>

<style scoped>

.login {
	width: 25rem;
}

form > div {
	margin-top: 1.5rem;
}

.login-btn {
	width: 100%;
	margin-top: 1rem;
}

@media (max-width: 60rem) {
	.login {
		width: 100%;
	}
}

</style>