import { createApp, ref, watch, inject, type Ref } from 'vue'
import vueCookies, { type VueCookies } from 'vue-cookies'
import PrimeVue from 'primevue/config'
import FocusTrap from 'primevue/focustrap'
import ToastService from 'primevue/toastservice'
import { registerSW } from 'virtual:pwa-register'
import App from './App.vue'
import router from './router'
import { useCookies } from './cookies'
import './utils/chart'
import { ping } from '@/api/v0'

import 'primevue/resources/themes/lara-light-green/theme.css'
import 'primeicons/primeicons.css'
import './assets/main.css'

registerSW({ immediate: true })

const app = createApp(App)

app.use(router)
app.use(vueCookies)

app.use(PrimeVue, { ripple: true })
app.use(ToastService)
app.directive('focustrap', FocusTrap)

const cookies = (app as unknown as { $cookies: VueCookies }).$cookies

const API_TOKEN_STORAGE_KEY = '_authToken'
const token: Ref<string | null> = ref(cookies.get(API_TOKEN_STORAGE_KEY))
watch(token, (value: string | null) => {
	if (value) {
		cookies.set(API_TOKEN_STORAGE_KEY, value, 60 * 60 * 10)
	} else {
		cookies.remove(API_TOKEN_STORAGE_KEY)
	}
})
if (token.value) {
	ping(token.value).then((pong) => {
		if (!pong.authed) {
			console.warn('Token expired')
			token.value = null
		}
	})
}
app.provide('token', token)

app.mount('#app')
