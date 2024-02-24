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

const emit = defineEmits<{
	logged: [token: string]
}>()

const username = ref('')
const password = ref('')
const errMsg = ref('')

async function login(): Promise<void> {
	errMsg.value = ''
	await nextTick()
	const user = username.value
	const passwd = sha256(password.value)
	if (!user) {
		errMsg.value = 'Please input the username'
		return
	}
	if (!passwd) {
		errMsg.value = 'Please input the password'
		return
	}
	password.value = ''
	const res = await axios
		.post(`/api/v0/login`, {
			username: user,
			password: passwd
		})
		.catch((err) => {
			console.log('err:', err)
			const data = err.response.data
			console.error('LoginError:', data)
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
	if (!res) {
		return
	}
	const { token: tk } = res.data as { token: string }
	emit('logged', tk)
}
</script>

<template>
	<div class="login">
		<form @submit.prevent="login">
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
					<label for="username">Username</label>
				</FloatLabel>
			</InputGroup>
			<InputGroup>
				<InputGroupAddon>
					<i class="pi pi-lock"></i>
				</InputGroupAddon>
				<FloatLabel>
					<Password
						name="password"
						v-model="password"
						:feedback="false"
						:inputProps="{ autocomplete: 'current-password' }"
					/>
					<label for="password">Password</label>
				</FloatLabel>
			</InputGroup>
			<Button class="login-btn" type="submit" label="Login" icon="pi pi-sign-in"/>
		</form>
		<Message v-if="errMsg" :closable="false" severity="error">
			{{errMsg}}
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
</style>