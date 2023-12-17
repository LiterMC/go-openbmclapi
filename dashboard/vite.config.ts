import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import pwa from './src/config/pwa'

// https://vitejs.dev/config/
export default defineConfig(async ({ command, mode }) => {
	console.log(command, mode)
	const isdev = mode === 'development'
	const minify = isdev ? '' : 'esbuild'

	return {
		plugins: [vue(), pwa],
		resolve: {
			alias: {
				'@': fileURLToPath(new URL('./src', import.meta.url)),
			},
		},
		mode: mode,
		base: '/dashboard',
		build: {
			minify: minify,
		},
		esbuild: {
			pure: isdev ? [] : ['console.debug'],
		},
	}
})
