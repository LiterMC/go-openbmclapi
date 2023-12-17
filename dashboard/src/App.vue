<script setup lang="ts">
import { computed } from 'vue'
import { RouterView } from 'vue-router'
import Dropdown from 'primevue/dropdown'
import { type Lang, avaliableLangs, getLang, setLang } from '@/lang'

const languages = avaliableLangs.map((v) => v.code)
const selectedLang = computed(() => getLang())

function updateLang(value: Lang){
	setLang(value)
}

const langNameMap: { [key: string]: string } = {
	'en-US': 'English',
	'zh-CN': '简体中文',
}

</script>

<template>
	<header id="header">
		<img class="header-logo" src="/logo.png" />
		<div class="lang-selector-box">
			<Dropdown :model-value="selectedLang" @update:model-value="updateLang" class="lang-selector"
				:options="languages" placeholder="Language">
				<template #value="slotProps">
					<span class="flex-row-center">
						<i class="pi pi-globe"></i>
						{{ langNameMap[slotProps.value.toString()] }}
					</span>
				</template>
				<template #option="slotProps">
					{{ langNameMap[slotProps.option.toString()] }}
				</template>
			</Dropdown>
		</div>
		<a class="nav-github" target="_blank" tabindex="-1" href="https://github.com/LiterMC/go-openbmclapi">
			<i class="pi pi-github"></i>
			<span>Github</span>
			<i class="pi pi-external-link"></i>
		</a>
	</header>
	<div id="body">
		<RouterView />
	</div>
	<footer id="footer">
		<p>
			Powered by <a href="https://vuejs.org/">Vue.js</a>
		</p>
		<address>
			<span>
				Author: <a href="mailto:zyxkad@gmail.com">Kevin Z &lt;zyxkad@gmail.com&gt;</a>
			</span>
		</address>
		<p>
			Copyright &copy; 2023 All rights reserved. <br/>
			This website is under <a href="https://www.gnu.org/licenses/agpl-3.0.html">AGPL-3.0 License</a>
		</p>
	</footer>
</template>

<style scoped>
#header {
	position: fixed;
	top: 0;
	left: 0;
	z-index: 999999;
	display: flex;
	flex-direction: row;
	align-items: center;
	justify-content: flex-end;
	width: 100vw;
	height: 4rem;
	padding: 0 2rem;
	background-color: color-mix(in srgb, var(--primary-50), transparent);
	box-shadow: #0008 0 0 1rem -0.5rem;
	backdrop-filter: blur(0.4rem);
}

.header-logo {
	width: 3rem;
	height: 3rem;
	position: absolute;
	left: 2rem;
}

.lang-selector .pi-globe {
	margin-right: 0.3rem;
}

.nav-github {
	display: flex;
	flex-direction: row;
	align-items: center;
	justify-content: space-evenly;
	height: 100%;
	margin-left: 1rem;
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

</style>
