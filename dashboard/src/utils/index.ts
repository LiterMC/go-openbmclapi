import { tr, getLang, Lang } from '@/lang'

const _EN = new Lang('en')
const _ZH = new Lang('zh')

export function formatNumber(num: number): string {
	const lang = getLang()
	var neg = ''
	if (num < 0) {
		neg = '-'
		num = -num
	}
	var res: string
	if (_ZH.match(lang)) {
		res = formatNumberZH(num)
	} else {
		res = formatNumberEN(num)
	}
	return neg + res
}

const nUnitsUS = ['k', 'm', 'B', 'T', 'Q']

function formatNumberEN(num: number): string {
	if (num < 1000) {
		return num.toString()
	}
	var unit
	for (const u of nUnitsUS) {
		unit = u
		num /= 1000
		if (num < 1000) {
			break
		}
	}
	return `${num.toFixed(2)} ${unit}`
}

const nUnitsZH = ['万', '亿', '兆', '京']

function formatNumberZH(num: number): string {
	if (num < 9000) {
		return num.toString()
	}
	var unit = ''
	for (const u of nUnitsZH) {
		unit = u
		num /= 10000
		if (num < 9000) {
			break
		}
	}
	return `${num.toFixed(2)} ${unit}`
}

const bUnits = ['KB', 'MB', 'GB', 'TB']

export function formatBytes(bytes: number): string {
	var neg = ''
	if (bytes < 0) {
		neg = '-'
		bytes = -bytes
	}
	if (bytes < 1000) {
		return `${neg}${bytes} B`
	}
	var unit = ''
	for (const u of bUnits) {
		unit = u
		bytes /= 1024
		if (bytes < 1000) {
			break
		}
	}
	return `${neg}${bytes.toFixed(2)} ${unit}`
}

export function formatTime(ms: number): string {
	var neg = ''
	if (ms < 0) {
		neg = '-'
		ms = -ms
	}
	var unit = tr('unit.time.ms')
	if (ms > 800) {
		ms /= 1000
		unit = tr('unit.time.s')
		if (ms > 50) {
			ms /= 60
			unit = tr('unit.time.min')
			if (ms > 50) {
				ms /= 60
				unit = tr('unit.time.hour')
				if (ms > 22) {
					ms /= 24
					unit = tr('unit.time.day')
					if (ms > 350) {
						ms /= 356
						unit = tr('unit.time.year')
					}
				}
			}
		}
	}
	return `${neg}${ms.toFixed(2)} ${unit}`
}
