import axios, { AxiosError } from 'axios'
import { sha256 } from 'js-sha256'

export interface StatInstData {
	hits: number
	bytes: number
}

// the range is from 0 to N
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
				'Content-Type': 'application/json',
			},
		},
	)
	return res.data.token
}

export async function ping(token?: string): Promise<PingRes> {
	const res = await axios.get<PingRes>(`/api/v0/ping`, {
		headers: {
			Authorization: token ? `Bearer ${token}` : undefined,
		},
	})
	return res.data
}

export async function getStatus(token?: string | null): Promise<StatusRes> {
	const res = await axios.get<StatusRes>(`/api/v0/status`, {
		headers: {
			Authorization: token ? `Bearer ${token}` : undefined,
		},
	})
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

interface SubscribeKey {
	publicKey: string
}

export async function getSubscribePublicKey(token?: string): Promise<string> {
	const res = await axios.get<SubscribeKey>(`/api/v0/subscribeKey`, {
		headers: {
			Authorization: token ? `Bearer ${token}` : undefined,
		},
	})
	return res.data.publicKey
}

export type SubscribeScope =
	| 'enabled'
	| 'disabled'
	| 'syncbegin'
	| 'syncdone'
	| 'updates'
	| 'dailyreport'
export const ALL_SUBSCRIBE_SCOPES: SubscribeScope[] = [
	'enabled',
	'disabled',
	'syncbegin',
	'syncdone',
	'updates',
	'dailyreport',
]
export type ScopeFlags = {
	[key in SubscribeScope]: boolean
}

export interface SubscribeSettings {
	scopes: ScopeFlags
	reportAt: string
}

export async function getSubscribeSettings(token: string): Promise<SubscribeSettings | null> {
	try {
		const res = await axios.get<SubscribeSettings>(`/api/v0/subscribe`, {
			headers: {
				Authorization: `Bearer ${token}`,
			},
		})
		return res.data
	} catch (e) {
		if (e instanceof AxiosError) {
			if (e.status == 404) {
				return null
			}
		}
		throw e
	}
}

export async function setSubscribeSettings(
	token: string,
	subscription: PushSubscription | null,
	scopes: SubscribeScope[],
	reportAt?: string | undefined,
): Promise<void> {
	await axios.post<SubscribeSettings>(
		`/api/v0/subscribe`,
		JSON.stringify({
			endpoint: subscription?.endpoint,
			keys: subscription?.toJSON().keys,
			scopes: scopes,
			reportAt: reportAt,
		}),
		{
			headers: {
				Authorization: `Bearer ${token}`,
				'Content-Type': 'application/json',
			},
		},
	)
}

export async function removeSubscription(token: string): Promise<void> {
	await axios.delete(`/api/v0/subscribe`, {
		headers: {
			Authorization: `Bearer ${token}`,
		},
	})
}

export interface EmailItemPayload {
	addr: string
	scopes: SubscribeScope[]
	enabled: boolean
}

export interface EmailItemRes {
	user: string
	addr: string
	scopes: ScopeFlags
	enabled: boolean
}

export async function getEmailSubscriptions(token: string): Promise<EmailItemRes[]> {
	const res = await axios.get<EmailItemRes[]>(`/api/v0/subscribe_email`, {
		headers: {
			Authorization: `Bearer ${token}`,
		},
	})
	return res.data
}

export async function addEmailSubscription(token: string, item: EmailItemPayload): Promise<void> {
	await axios.post(`/api/v0/subscribe_email`, JSON.stringify(item), {
		headers: {
			Authorization: `Bearer ${token}`,
			'Content-Type': 'application/json',
		},
	})
}

export async function updateEmailSubscription(
	token: string,
	addr: string,
	item: EmailItemPayload,
): Promise<void> {
	await axios.post(`/api/v0/subscribe_email`, JSON.stringify(item), {
		params: {
			addr: addr,
		},
		headers: {
			Authorization: `Bearer ${token}`,
			'Content-Type': 'application/json',
		},
	})
}

export async function removeEmailSubscription(token: string, addr: string): Promise<void> {
	await axios.delete(`/api/v0/subscribe_email`, {
		params: {
			addr: addr,
		},
		headers: {
			Authorization: `Bearer ${token}`,
		},
	})
}

export interface WebhookItemPayload {
	name: string
	endpoint: string
	auth: string | undefined
	scopes: SubscribeScope[]
	enabled: boolean
}

export interface WebhookItemRes {
	id: number
	name: string
	endpoint: string
	authHash?: string
	scopes: ScopeFlags
	enabled: boolean
}

export async function getWebhooks(token: string): Promise<WebhookItemRes[]> {
	const res = await axios.get<WebhookItemRes[]>(`/api/v0/webhook`, {
		headers: {
			Authorization: `Bearer ${token}`,
		},
	})
	return res.data
}

export async function addWebhook(token: string, item: WebhookItemPayload): Promise<void> {
	await axios.post(`/api/v0/webhook`, JSON.stringify(item), {
		headers: {
			Authorization: `Bearer ${token}`,
			'Content-Type': 'application/json',
		},
	})
}

export async function updateWebhook(
	token: string,
	id: number,
	item: WebhookItemPayload,
): Promise<void> {
	await axios.patch(`/api/v0/webhook`, JSON.stringify(item), {
		params: {
			id: id,
		},
		headers: {
			Authorization: `Bearer ${token}`,
			'Content-Type': 'application/json',
		},
	})
}

export async function removeWebhook(token: string, id: number): Promise<void> {
	await axios.delete(`/api/v0/webhook`, {
		params: {
			id: id,
		},
		headers: {
			Authorization: `Bearer ${token}`,
		},
	})
}
