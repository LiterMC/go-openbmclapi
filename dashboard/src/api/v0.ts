export interface StatInstData {
	hits: number
	bytes: number
}

export interface StatTime {
	hour: number
	day: number
	month: number
	year: number
}

export interface StatHistoryData {
	hours: StatInstData[]
	days: StatInstData[]
	months: StatInstData[]
}

export type Stats = StatHistoryData & {
	date: StatTime
	prev: StatHistoryData
	years: { [key: string]: StatInstData }
}

export interface APIStatus {
	startAt: string
	stats: Stats
	enabled: boolean
	accesses: { [ua: string]: number }
}
