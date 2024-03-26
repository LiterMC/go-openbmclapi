<script setup lang="ts">
import { inject, ref, nextTick, type Ref } from 'vue'
import axios from 'axios'
import Message from 'primevue/message'
import FloatLabel from 'primevue/floatlabel'
import InputGroup from 'primevue/inputgroup'
import InputGroupAddon from 'primevue/inputgroupaddon'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Button from 'primevue/button'
import { login as apiLogin } from '@/api/v0'
import { tr } from '@/lang'

const emit = defineEmits<{
	logged: [token: string]
}>()

const username = ref('')
const password = ref('')
const usernameInvalid = ref(false)
const passwordInvalid = ref(false)
const loading = ref(false)
const errMsg = ref<(() => string) | string | null>(null)

async function login(): Promise<void> {
	usernameInvalid.value = false
	passwordInvalid.value = false
	errMsg.value = null
	await nextTick()

	const user = username.value
	const passwd = password.value
	if (!user) {
		usernameInvalid.value = true
		errMsg.value = () => tr('message.login.input.username')
		return
	}
	if (!password.value) {
		passwordInvalid.value = true
		errMsg.value = () => tr('message.login.input.password')
		return
	}
	loading.value = true
	const token = await apiLogin(user, passwd).catch((err) => {
		console.error('LoginError:', err)
		const code = err.response?.status
		if (code === 429) {
			errMsg.value = 'Too many requests, please try again later'
			return null
		}
		const data = err.response?.data
		if (data?.error) {
			if (data.error.indexOf(' incorrect') >= 0) {
				usernameInvalid.value = true
				passwordInvalid.value = true
			}
			errMsg.value = 'LoginError: ' + data.error
			if (data.message) {
				errMsg.value += ': ' + data.message
			}
		} else {
			errMsg.value = String(err)
		}
		return null
	})
	loading.value = false
	if (!token) {
		return
	}
	emit('logged', token)
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
						:invalid="usernameInvalid"
					/>
					<label for="username">{{ tr('title.username') }}</label>
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
						:invalid="passwordInvalid"
					/>
					<label for="password">{{ tr('title.password') }}</label>
				</FloatLabel>
			</InputGroup>
			<Button
				class="login-btn"
				type="submit"
				:label="tr('title.login')"
				:loading="loading"
				icon="pi pi-sign-in"
			/>
		</form>
		<Message v-if="errMsg" :closable="false" severity="error">
			{{ typeof errMsg === 'function' ? errMsg() : errMsg }}
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
