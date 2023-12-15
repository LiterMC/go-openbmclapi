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
