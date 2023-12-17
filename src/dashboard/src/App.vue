<script setup lang="ts">
import { computed } from 'vue'
import { RouterView } from 'vue-router'
import Dropdown from 'primevue/dropdown'
import { avaliableLangs, getLang, setLang } from '@/lang'

const languages = avaliableLangs.map((v) => v.code)
const selectedLang = computed(() => getLang())

function updateLang(value){
	setLang(value)
}

</script>

<template>
	<header id="header">
		<div class="lang-selector-box">
			<Dropdown :model-value="selectedLang" @update:model-value="updateLang" class="lang-selector"
				:options="languages" placeholder="Language" />
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
</template>

<style scoped>
#header {
	position: fixed;
	top: 0;
	left: 0;
	display: flex;
	flex-direction: row;
	align-items: center;
	justify-content: flex-end;
	width: 100vw;
	height: 4rem;
	padding: 0 2rem;
	background-color: #f8fffe;
	box-shadow: #0008 0 0 1rem -0.5rem;
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
</style>
