import { createApp, ref, watch, inject, type Ref } from 'vue'
import vueCookies, { type VueCookies } from 'vue-cookies'
import PrimeVue from 'primevue/config'
import FocusTrap from 'primevue/focustrap'
import ToastService from 'primevue/toastservice'
import { registerSW } from 'virtual:pwa-register'
import App from './App.vue'
import router from './router'
import { useCookies, bindRefToCookie } from './cookies'
import './utils/chart'
import { ping } from '@/api/v0'

import 'primevue/resources/themes/lara-light-green/theme.css'
import 'primeicons/primeicons.css'
import './assets/main.css'

registerSW({
	immediate: true,
	async onRegisteredSW(
		swScriptUrl: string,
		registration: ServiceWorkerRegistration | undefined,
	): Promise<void> {
		if (!registration) {
			return
		}
		// registration.sync.register('poll-state')
	},
})

const app = createApp(App)

app.use(router)
app.use(vueCookies, { expires: '30d', path: import.meta.env.BASE_URL })

app.use(PrimeVue, { ripple: true })
app.use(ToastService)
app.directive('focustrap', FocusTrap)

const cookies = (app as unknown as { $cookies: VueCookies }).$cookies

const API_TOKEN_STORAGE_KEY = '_authToken'
const token: Ref<string | null> = bindRefToCookie(
	ref(null),
	API_TOKEN_STORAGE_KEY,
	60 * 60 * 12,
	cookies,
)

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
