module.exports = {
	pwa: {
		name: 'Go-OpenBmclApi Dashboard',
		themeColor: '#4c89fe',
		msTileColor: '#4c89fe',
		manifestOptions: {
			start_url: '.',
			background_color: '#4c89fe'
		},
		workboxPluginMode: 'GenerateSW',
		workboxOptions: {
		},
		publicPath: '/dashboard',
	}
}