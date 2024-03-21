import { reactive, inject, watch, type Ref } from 'vue'
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
		if (value === undefined) {
			localStorage.removeItem(name)
		} else {
			localStorage.setItem(name, JSON.stringify(value))
		}
	})
	return ref
}

export function bindObjectToLocalStorage<T extends object>(obj: T, name: string): T {
	const active = reactive(obj) as T
	const s = localStorage.getItem(name)
	if (s) {
		try {
			const parsed = JSON.parse(s)
			for (const k of Object.keys(parsed)) {
				if (k in active) {
					active[k as keyof T] = parsed[k as keyof T]
				}
			}
		} catch (_) {}
	}
	watch(active, () => {
		localStorage.setItem(name, JSON.stringify(active))
	})
	return active
}
