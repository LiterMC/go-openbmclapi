import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig(async ({ command, mode }) => {
	console.log(command, mode)
	const isdev = mode === 'development'
	const minify = isdev ? '' : 'esbuild'

	return {
		plugins: [vue()],
		resolve: {
			alias: {
				'@': fileURLToPath(new URL('./src', import.meta.url)),
			},
		},
		mode: mode,
		base: '/main',
		build: {
			minify: minify,
		},
		esbuild: {
			pure: isdev ? [] : ['console.debug'],
		},
	}
})
