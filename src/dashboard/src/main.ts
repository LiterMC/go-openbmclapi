import { createApp } from 'vue'
import PrimeVue from 'primevue/config'
import { registerSW } from 'virtual:pwa-register'
import App from './App.vue'
import router from './router'
import './utils/chart'

import 'primevue/resources/themes/lara-light-green/theme.css'
import './assets/main.css'

registerSW({ immediate: true })

const app = createApp(App)

app.use(router)
app.use(PrimeVue, { ripple: true })

app.mount('#app')
