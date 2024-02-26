<script setup lang="ts">
import { computed, inject, nextTick, type Ref } from 'vue'
import { RouterLink, RouterView } from 'vue-router'
import axios from 'axios'
import Button from 'primevue/button'
import Dropdown from 'primevue/dropdown'
import Toast from 'primevue/toast'
import { useToast } from 'primevue/usetoast'
import { type Lang, avaliableLangs, getLang, setLang, tr } from '@/lang'

const toast = useToast()
const token = inject('token') as Ref<string | null>

async function logout(): Promise<void> {
	try {
		await axios.post(`/api/v0/logout`, null, {
			headers: {
				Authorization: `Bearer ${token.value}`,
			},
		})
		token.value = null
		await nextTick() // wait for token to be cleared
		window.location.reload()
	} catch (err) {
		toast.add({ severity: 'error', summary: 'Cannot logout', detail: String(err), life: 3000 })
		token.value = null
	}
}

const languages = avaliableLangs.map((v) => v.code)
const selectedLang = computed({
	get() {
		return getLang()
	},
	set(value) {
		setLang(value)
	},
})

const langNameMap: { [key: string]: string } = {
	'en-US': 'English',
	'zh-CN': '简体中文',
}
</script>

<template>
	<header id="header" class="flex-row-center">
		<RouterLink class="flex-row-center header-logo-box" to="/">
			<img class="header-logo" src="/logo.png" />
		</RouterLink>

		<div v-if="token" class="flex-row-center no-select nav-login pointer" @click="logout">
			<span>{{ tr('title.logout') }}&nbsp;</span>
			<i class="pi pi-sign-out"></i>
		</div>
		<RouterLink v-else class="flex-row-center no-select nav-login" to="/login">
			<span>{{ tr('title.login') }}&nbsp;</span>
			<i class="pi pi-sign-in"></i>
		</RouterLink>

		<div class="lang-selector-box">
			<Dropdown
				v-model="selectedLang"
				class="lang-selector"
				:options="languages"
				placeholder="Language"
			>
				<template #value="slotProps">
					<span class="flex-row-center lang-selector-label" style="margin-right: -0.75rem">
						<i class="pi pi-globe"></i>
						{{ langNameMap[slotProps.value.toString()] }}
					</span>
				</template>
				<template #option="slotProps">
					{{ langNameMap[slotProps.option.toString()] }}
				</template>
			</Dropdown>
		</div>
		<a
			class="nav-github"
			target="_blank"
			tabindex="-1"
			href="https://github.com/LiterMC/go-openbmclapi?tab=readme-ov-file#go-openbmclapi"
		>
			<i class="pi pi-github"></i>
			<span>Github</span>
			<i class="pi pi-external-link"></i>
		</a>
	</header>
	<div id="body">
		<RouterView />
	</div>
	<footer id="footer">
		<p>Powered by <a href="https://vuejs.org/">Vue.js</a></p>
		<address>
			<span> Author: <a href="mailto:zyxkad@gmail.com">Kevin Z &lt;zyxkad@gmail.com&gt;</a> </span>
		</address>
		<p>
			Copyright &copy; 2023-2024 All rights reserved. <br />
			This website is under
			<a href="https://www.gnu.org/licenses/agpl-3.0.html">AGPL-3.0 License</a>
		</p>
	</footer>
	<Toast position="bottom-right" />
</template>

<style scoped>
#header {
	position: fixed;
	top: 0;
	left: 0;
	z-index: 999999;
	justify-content: flex-end;
	width: 100vw;
	height: 4rem;
	padding-left: 2rem;
	padding-right: 1rem;
	background-color: color-mix(in srgb, var(--primary-50), transparent);
	box-shadow: #0008 0 0 1rem -0.5rem;
	backdrop-filter: blur(0.4rem);
}

#header > * {
	margin-right: 1rem;
}

.header-logo-box {
	position: absolute;
	left: 2rem;
}

.header-logo {
	width: 3rem;
	height: 3rem;
}

.nav-login {
	color: var(--primary-color);
	text-decoration: none;
}

.lang-selector .pi-globe {
	margin-right: 0.3rem;
}

.lang-selector-label {
	font-size: 0.9rem;
}

.nav-github {
	display: flex;
	flex-direction: row;
	align-items: center;
	justify-content: space-evenly;
	height: 100%;
	color: #70a3f5;
	text-decoration: none;
	cursor: pointer;
	user-select: none;
	transition: 0.2s color ease;
}

.nav-github:hover {
	color: #000;
}

.nav-github .pi-github {
	margin-right: 0.2rem;
}

.nav-github .pi-external-link {
	margin-left: 0.2rem;
	font-size: 0.5rem;
	height: 1.2rem;
}

#body {
	padding: 5rem 2rem 4rem 2rem;
	min-height: 100vh;
}

#footer {
	padding: 2rem;
	font-family: var(--font-family);
	color: var(--primary-color-text);
	background-color: var(--primary-color);
}

#footer > * {
	margin: 0;
}

#footer a {
	color: var(--primary-100);
	text-decoration: none;
	transition: 0.4s color, 0.2s background-color ease;
}

#footer a:hover {
	color: var(--highlight-text-color);
	background-color: var(--highlight-bg);
	text-decoration: underline;
}

@media (max-width: 60rem) {
	#header {
		padding: 0;
	}

	#header > * {
		margin-right: 0.6rem;
	}

	.header-logo-box {
		left: 1rem;
	}

	.lang-selector-label {
		font-size: 0.8rem;
	}
}
</style>
