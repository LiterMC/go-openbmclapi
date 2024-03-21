import axios from 'axios'
import { sha256 } from 'js-sha256'

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

	accesses: { [ua: string]: number }
}

export interface TokenRes {
	token: string
}

export interface PingRes {
	version: string
	time: string
	authed: boolean
}

export interface StatusRes {
	startAt: string
	stats: Stats
	enabled: boolean
	isSync: boolean
	sync?: {
		prog: number
		total: number
	}
}

async function requestToken(
	token: string,
	path: string,
	query?: { [key: string]: string },
): Promise<string> {
	const res = await axios.post<TokenRes>(
		`/api/v0/requestToken`,
		JSON.stringify({
			path: path,
			query: query,
		}),
		{
			headers: {
				Authorization: `Bearer ${token}`,
			},
		},
	)
	return res.data.token
}

export async function ping(token?: string): Promise<PingRes> {
	const res = await axios.get<PingRes>(`/api/v0/ping`, {
		headers: token
			? {
					Authorization: `Bearer ${token}`,
			  }
			: undefined,
	})
	return res.data
}

export async function getStatus(): Promise<StatusRes> {
	const res = await axios.get<StatusRes>(`/api/v0/status`)
	return res.data
}

export async function login(username: string, password: string): Promise<string> {
	const res = await axios.post<TokenRes>(`/api/v0/login`, {
		username: username,
		password: sha256(password),
	})
	return res.data.token
}

// Avaliable values for pprof lookup are at <https://pkg.go.dev/runtime/pprof>
//
// goroutine    - stack traces of all current goroutines
// heap         - a sampling of memory allocations of live objects
// allocs       - a sampling of all past memory allocations
// threadcreate - stack traces that led to the creation of new OS threads
// block        - stack traces that led to blocking on synchronization primitives
// mutex        - stack traces of holders of contended mutexes
export type PprofLookups = 'goroutine' | 'heap' | 'allocs' | 'threadcreate' | 'block' | 'mutex'

export interface PprofOptions {
	lookup: PprofLookups
	// 'view' default is true
	view?: boolean
	// 'debug' default is true
	debug?: boolean
}

export async function getPprofURL(token: string, opts: PprofOptions): Promise<string> {
	const pprofURL = `/api/v0/pprof`
	const tk = await requestToken(token, pprofURL, {
		lookup: opts.lookup,
	})
	const u = new URL(window.location.toString())
	u.pathname = pprofURL
	u.searchParams.set('lookup', opts.lookup)
	u.searchParams.set('_t', tk)
	if (opts.debug === false) {
		u.searchParams.set('debug', '0')
	} else {
		u.searchParams.set('debug', '1')
		if (opts.view !== false) {
			u.searchParams.set('view', '1')
		}
	}
	return u.toString()
}

export type SubscribeScope = 'disabled' | 'enabled' | 'syncdone' | 'updates'

export interface SubscribeSettings {
	subscribed: SubscribeScope[] | null
}

export async function getSubscribeSettings(token: string): Promise<SubscribeSettings> {
	const res = await axios.get<SubscribeSettings>(`/api/v0/settings/subscribe`, {
		headers: {
			Authorization: `Bearer ${token}`,
		},
	})
	return res.data
}
