import {
	cleanupOutdatedCaches,
	createHandlerBoundToURL,
	precacheAndRoute,
} from 'workbox-precaching'
import { clientsClaim } from 'workbox-core'
import { NavigationRoute, registerRoute } from 'workbox-routing'
import pako from 'pako'
import type { Stats } from '@/api/v0'
import { setLang } from '@/lang'
import { formatBytes, formatNumber } from '@/utils'

declare let self: ServiceWorkerGlobalScope

const BASE_URL = (() => {
	return import.meta.env.BASE_URL.endsWith('/')
		? import.meta.env.BASE_URL.substr(0, import.meta.env.BASE_URL.length - 1)
		: import.meta.env.BASE_URL
})()
const ICON_URL = BASE_URL + '/favicon.ico'

console.log('PWA Service worker loading')

setLang('zh-CN') // TODO: do not hardcode translations

// self.__WB_MANIFEST is default injection point
precacheAndRoute(self.__WB_MANIFEST)

// clean old assets
cleanupOutdatedCaches()

if (!import.meta.env.DEV) {
	// to allow work offline
	registerRoute(new NavigationRoute(createHandlerBoundToURL('index.html')))
}

self.skipWaiting()
clientsClaim()

async function onSyncUserSettings(): Promise<void> {
	// axios.get()
}

type PushData =
	| {
			typ: 'enabled' | 'disabled' | 'syncdone'
			at: number
	  }
	| {
			typ: 'updates'
			tag: string
	  }
	| {
			typ: 'daily-report'
			data: string
	  }

function decodeB64(b64: string): Uint8Array {
	const bin = atob(b64)
	const bts = new Uint8Array(bin.length)
	for (var i = 0; i < bin.length; i++) {
		bts[i] = bin.charCodeAt(i)
	}
	return bts
}

const bUnits = ['KB', 'MB', 'GB', 'TB']

async function onRecvPush(data: PushData): Promise<void> {
	switch (data.typ) {
		case 'enabled':
		case 'disabled':
		case 'syncdone':
			await self.registration
				.showNotification('OpenBmclApi', {
					icon: ICON_URL,
					tag: `status-${data.typ}`,
					body: `Cluster ${data.typ}`,
				})
				.catch((err) => console.error('notify error:', err))
			break
		case 'updates':
			await self.registration
				.showNotification('OpenBmclApi', {
					icon: ICON_URL,
					tag: 'update-notify',
					body: `New version (${data.tag}) avaliable`,
				})
				.catch((err) => console.error('notify error:', err))
			break
		case 'daily-report': {
			const compressed = decodeB64(data.data)
			const stats: Stats = JSON.parse(pako.inflate(compressed, { to: 'string' }))
			console.debug('daily-report stats:', stats)
			const lastDay = new Date(
				Date.UTC(stats.date.year, stats.date.month + 1, stats.date.day, stats.date.hour),
			)
			const lastTwoDay = new Date(
				Date.UTC(stats.date.year, stats.date.month + 1, stats.date.day - 1, stats.date.hour),
			)
			const lastStat =
				lastDay.getUTCMonth() === stats.date.month + 1
					? stats.days[lastDay.getUTCDate() - 1]
					: stats.prev.days[lastDay.getUTCDate() - 1]
			const lastTwoStat =
				lastTwoDay.getUTCMonth() === stats.date.month + 1
					? stats.days[lastTwoDay.getUTCDate() - 1]
					: stats.prev.days[lastTwoDay.getUTCDate() - 1]
			await self.registration
				.showNotification('OpenBmclApi', {
					icon: ICON_URL,
					tag: `daily-report`,
					body: `昨日数据: 流量 ${formatBytes(lastStat.bytes)}, 请求 ${formatNumber(
						lastStat.hits,
					)}\n增长 ${formatBytes(lastStat.bytes - lastTwoStat.bytes)}`,
				})
				.catch((err) => console.error('notify error:', err))
			break
		}
	}
}

async function onNotificationClick(tag: string): Promise<void> {
	switch (tag) {
		case 'updates': {
			self.clients.openWindow('https://github.com/LiterMC/go-openbmclapi/releases')
			return
		}
		default: {
			const windowClients = await self.clients.matchAll({ type: 'window' })
			for (const client of windowClients) {
				client.focus()
				return
			}
			self.clients.openWindow(import.meta.env.BASE_URL)
			return
		}
	}
}

self.addEventListener('sync', (event: SyncEvent) => {
	switch (event.tag) {
		case 'user-settings':
			event.waitUntil(onSyncUserSettings())
			break
	}
})

self.addEventListener('push', (event: PushEvent) => {
	if (!event.data) {
		return
	}
	const data = event.data.json()
	event.waitUntil(onRecvPush(data))
})

self.addEventListener('notificationclick', (event: NotificationEvent) => {
	event.notification.close()
	event.waitUntil(onNotificationClick(event.notification.tag))
})

// // @ts-expect-error TS2769
// self.addEventListener('periodicsync', (event: SyncEvent) => {
// 	event.waitUntil(
// 		self.registration
// 			.showNotification('OpenBmclApi', {
// 				icon: import.meta.env.BASE_URL + '/favicon.ico',
// 				body: `periodicsync triggered at ${new Date().toString()} for ${event.tag}`,
// 			})
// 			.catch((err) => console.error(err)),
// 	)
// })
