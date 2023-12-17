import { type Ref, ref } from 'vue'
import { Lang } from './lang'
export * from './lang'

// keys use with window.localStorage
const TR_LANG_CACHE_KEY = 'go-openbmclapi.dashboard.tr.lang'
const TR_DATA_CACHE_KEY = 'go-openbmclapi.dashboard.tr.map'

interface LangMap {
	[key: string]: string | LangMap
}

interface langItem {
	code: Lang
	tr: () => Promise<LangMap>
}

export const avaliableLangs = [
	{ code: new Lang('en-US'), tr: () => import('@/assets/lang/en-US.json') },
	{ code: new Lang('zh-CN'), tr: () => import('@/assets/lang/zh-CN.json') },
]

export const defaultLang = avaliableLangs[0]
const currentLang = ref(defaultLang)
const currentTr: Ref<LangMap | null> = ref(null)

;(async function () {
	const langCache = localStorage.getItem(TR_LANG_CACHE_KEY)
	if(langCache){
		for (let a of avaliableLangs) {
			if (a.code.match(langCache)) {
				currentLang.value = a
				localStorage.setItem(TR_LANG_CACHE_KEY, langCache)
				break
			}
		}
	}
	try {
		// use local cache before translate map loaded then refresh will not always flash words
		const data = JSON.parse(localStorage.getItem(TR_DATA_CACHE_KEY) as string)
		if (typeof data === 'object') {
			currentTr.value = data as LangMap
		}
	} catch {}
	currentTr.value = await currentLang.value.tr()
	localStorage.setItem(TR_DATA_CACHE_KEY, JSON.stringify(currentTr.value))
})()

export function getLang(): Lang {
	return currentLang.value.code
}

export async function setLang(lang: Lang | string): Promise<Lang | null> {
	for (let a of avaliableLangs) {
		if (a.code.match(lang)) {
			localStorage.setItem(TR_LANG_CACHE_KEY, a.code.toString())
			currentLang.value = a
			currentTr.value = await a.tr()
			localStorage.setItem(TR_DATA_CACHE_KEY, JSON.stringify(currentTr.value))
			return a.code
		}
	}
	return null
}

export function tr(key: string, ...values: any[]): string {
	const item = currentLang.value
	let cur: string | LangMap | null = currentTr.value
	if (!cur) {
		return `{{${key}}}`
	}
	let keys = key.split('.')
	for (let k of keys) {
		if (!cur || typeof cur === 'string') {
			return `{{${key}}}`
		}
		cur = cur[k]
	}
	if (typeof cur !== 'string') {
		return `{{${key}}}`
	}
	// TODO: apply values
	return cur
}
