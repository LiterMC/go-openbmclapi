import { VitePWA } from 'vite-plugin-pwa'

export default VitePWA({
	registerType: 'autoUpdate',
	injectRegister: 'inline',
	workbox: {
		globPatterns: ['**/*.{js,css,html,ico,png,svg}'],
	},
	includeAssets: ['favicon.ico', 'apple-touch-icon.png'],
	manifest: {
		name: 'GoOpemBmclApi Dashboard',
		short_name: 'GoOpemBmclApiDashboard',
		description: 'Go-Openbmclapi Internal Dashboard',
		theme_color: '#4c89fe',
		icons: [
			{
				src: 'pwa-64x64.png',
				sizes: '64x64',
				type: 'image/png',
			},
			{
				src: 'pwa-192x192.png',
				sizes: '192x192',
				type: 'image/png',
			},
			{
				src: 'pwa-512x512.png',
				sizes: '512x512',
				type: 'image/png',
				purpose: 'any',
			},
			{
				src: 'maskable-icon-512x512.png',
				sizes: '512x512',
				type: 'image/png',
				purpose: 'maskable',
			},
		],
	},
})
