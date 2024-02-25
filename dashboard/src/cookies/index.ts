import { inject, watch, type Ref } from 'vue'
import type { VueCookies } from 'vue-cookies'

export function useCookies(): VueCookies {
	return inject('$cookies') as VueCookies
}

export function bindRefToCookie(
	ref: Ref<string | null>,
	name: string,
	expires: number,
	cookies?: VueCookies,
): Ref<string | null> {
	const c = cookies || useCookies()
	watch(ref, (value: string | null) => {
		if (value) {
			c.set(name, value, expires)
		} else {
			c.remove(name)
		}
	})
	ref.value = c.get(name)
	return ref
}

export function bindRefToLocalStorage<T>(
	ref: Ref<T | undefined>,
	name: string,
): Ref<T | undefined> {
	const s = localStorage.getItem(name)
	if (s) {
		try {
			ref.value = JSON.parse(s)
		} catch (_) {}
	}
	watch(ref, (value: T | undefined) => {
		if (value) {
			localStorage.setItem(name, JSON.stringify(value))
		} else {
			localStorage.removeItem(name)
		}
	})
	return ref
}
