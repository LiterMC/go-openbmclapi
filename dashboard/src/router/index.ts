import { createRouter, createWebHistory } from 'vue-router'
import HomeView from '@/views/HomeView.vue'

const router = createRouter({
	history: createWebHistory(import.meta.env.BASE_URL),
	routes: [
		{
			path: '/',
			name: 'home',
			component: HomeView,
		},
		{
			path: '/about',
			name: 'about',
			// route level code-splitting
			// this generates a separate chunk (About.[hash].js) for this route
			// which is lazy-loaded when the route is visited.
			component: () => import('@/views/AboutView.vue'),
		},
		{
			path: '/login',
			name: 'login',
			component: () => import('@/views/LoginView.vue'),
			props: (route) => ({ next: route.query.next }),
		},
		{
			path: '/settings',
			name: 'settings',
			component: () => import('@/views/SettingsView.vue'),
		},
		{
			path: '/loglist',
			name: 'loglist',
			component: () => import('@/views/LogListView.vue'),
		},
		{
			path: '/settings/notifications',
			name: 'settings/notifications',
			component: () => import('@/views/settings/NotificationsView.vue'),
		},
	],
	scrollBehavior(to, from, savedPosition) {
		return savedPosition || { top: 0 }
	},
})

export default router
