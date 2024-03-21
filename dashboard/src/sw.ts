import {
	cleanupOutdatedCaches,
	createHandlerBoundToURL,
	precacheAndRoute,
} from 'workbox-precaching'
import { clientsClaim } from 'workbox-core'
import { NavigationRoute, registerRoute } from 'workbox-routing'

declare let self: ServiceWorkerGlobalScope

console.log('PWA Service worker loading')

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

async function onNotificationClick(tag: string): Promise<void> {
	switch (tag) {
		case 'sync': {
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
