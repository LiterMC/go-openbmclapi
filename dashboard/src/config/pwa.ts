import { VitePWA } from 'vite-plugin-pwa'

export default VitePWA({
	registerType: 'autoUpdate',
	injectRegister: 'auto',
	includeAssets: ['favicon.ico'],
	manifest: {
		name: 'GoOpenBmclApi Dashboard',
		short_name: 'GOBA Dash',
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
	workbox: {
		globPatterns: ['**/*.{js,css,html,ico,png,svg,woff2}'],
	},
})
