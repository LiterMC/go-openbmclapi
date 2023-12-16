const nUnits = ['k', 'm', 'B', 'T', 'Q']

export function formatNumber(num: number): string {
	if (num < 1000) {
		return num.toString()
	}
	var unit
	for (const u of nUnits) {
		unit = u
		num /= 1000
		if (num < 1000) {
			break
		}
	}
	return `${num.toFixed(2)} ${unit}`
}

const bUnits = ['KB', 'MB', 'GB', 'TB']

export function formatBytes(bytes: number): string {
	if (bytes < 1000) {
		return bytes.toString()
	}
	var unit
	for (const u of bUnits) {
		unit = u
		bytes /= 1024
		if (bytes < 1000) {
			break
		}
	}
	return `${bytes.toFixed(2)} ${unit}`
}

export function formatTime(ms: number): string {
	var unit = 'ms'
	if (ms > 800) {
		ms /= 1000
		unit = 's'
		if (ms > 50) {
			ms /= 60
			unit = 'min'
			if (ms > 50) {
				ms /= 60
				unit = 'hour'
				if (ms > 22) {
					ms /= 24
					unit = 'day'
					if (ms > 350) {
						ms /= 356
						unit = 'year'
					}
				}
			}
		}
	}
	return `${ms.toFixed(2)} ${unit}`
}
