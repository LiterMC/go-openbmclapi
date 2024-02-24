import { inject } from 'vue'
import type { VueCookies } from 'vue-cookies'

export function useCookies(): VueCookies {
	return inject('$cookies') as VueCookies
}
