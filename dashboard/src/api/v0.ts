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

const EMPTY_HIST_STAT: StatHistoryData = {
	hours: Array(24).fill({ hits: 0, bytes: 0 }),
	days: Array(31).fill({ hits: 0, bytes: 0 }),
	months: Array(12).fill({ hits: 0, bytes: 0 }),
}

export const EMPTY_STAT: Stats = {
	...EMPTY_HIST_STAT,
	date: {
		hour: 0,
		day: 0,
		month: 0,
		year: 0,
	},
	prev: EMPTY_HIST_STAT,
	years: {},
	accesses: {},
}

interface ChallengeRes {
	token: string
}

export interface TokenRes {
	token: string
}

export enum UserPermission {
	BASIC = 1 << 0,
	SUBSCRIBE = 1 << 1,
	LOG = 1 << 2,
	DEBUG = 1 << 3,
	FULL_CONFIG = 1 << 4,
	CLUSTER = 1 << 5,
	STORAGE = 1 << 6,
	BYPASS_LIMIT = 1 << 7,
	ROOT = 1 << 31,
}

export interface UserInfoRes {
	name: string
	permissions: number
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
	storages: string[]
}

async function requestToken(
	token: string,
	path: string,
	query?: { [key: string]: string | undefined },
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

export async function getUserInfo(token: string): Promise<UserInfoRes> {
	const res = await axios.get<UserInfoRes>(`/api/v0/user_info`, {
		headers: {
			Authorization: `Bearer ${token}`,
		},
	})
	return res.data
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

export async function getStat(name: string, token?: string | null): Promise<Stats | null> {
	const res = await axios.get<Stats | null>(`/api/v0/stat/storage/${name}`, {
		headers: {
			Authorization: token ? `Bearer ${token}` : undefined,
		},
	})
	return res.data
}

export async function getChallenge(action: string): Promise<string> {
	const u = new URL(window.location.origin)
	u.pathname = `/api/v0/challenge`
	u.searchParams.set('action', action)
	const res = await axios.get<ChallengeRes>(u.toString())
	const token = res.data.token
	const body = JSON.parse(atob(token.split('.')[1]))
	if (body.act !== action) {
		throw new Error(`Challenge action not match, got ${body.act}, expect ${action}`)
	}
	return token
}

export function signChallenge(challenge: string, secret: string): string {
	const signed = sha256.hmac(sha256(secret), challenge)
	return signed
}

export async function login(username: string, password: string): Promise<string> {
	const challenge = await getChallenge('login')
	const signature = signChallenge(challenge, password)
	const res = await axios.post<TokenRes>(`/api/v0/login`, {
		username: username,
		challenge: challenge,
		signature: signature,
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
	const PPROF_URL = `/api/v0/pprof`
	const tk = await requestToken(token, PPROF_URL, {
		lookup: opts.lookup,
	})
	const u = new URL(window.location.origin)
	u.pathname = PPROF_URL
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

export interface FileInfo {
	name: string
	size: number
}

interface LogFilesRes {
	files: FileInfo[]
}

export async function getLogFiles(token: string): Promise<FileInfo[]> {
	const res = await axios.get<LogFilesRes>(`/api/v0/log_files`, {
		headers: {
			Authorization: `Bearer ${token}`,
		},
	})
	return res.data.files
}

export async function getLogFile(
	token: string,
	name: string,
	noEncrypt?: boolean,
): Promise<ArrayBuffer> {
	const LOGFILE_URL = `${window.location.origin}/api/v0/log_file/`
	const u = new URL(name.startsWith('/') ? name.substr(1) : name, LOGFILE_URL)
	if (noEncrypt) {
		u.searchParams.set('no_encrypt', '1')
	}
	const res = await axios.get<ArrayBuffer>(u.toString(), {
		headers: {
			Authorization: `Bearer ${token}`,
		},
		responseType: 'arraybuffer',
	})
	return res.data
}

export async function getLogFileURL(
	token: string,
	name: string,
	noEncrypt?: boolean,
): Promise<string> {
	const LOGFILE_URL = `${window.location.origin}/api/v0/log_file/`
	const u = new URL(name.startsWith('/') ? name.substr(1) : name, LOGFILE_URL)
	if (noEncrypt) {
		u.searchParams.set('no_encrypt', '1')
	}
	const tk = await requestToken(token, u.pathname, {
		no_encrypt: u.searchParams.get('no_encrypt') || '',
	})
	u.searchParams.set('_t', tk)
	return u.toString()
}

export interface Config {
	public_host: string
	public_port: number
	host: string
	port: number
	use_cert: boolean
	allow_unsecure_connection: boolean
	trusted_x_forwarded_for: boolean

	only_gc_when_start: boolean
	sync_interval: number
	download_max_conn: number
	max_reconnect_count: number

	log_slots: number
	no_access_log: boolean
	access_log_slots: number

	clusters: { [name: string]: ClusterOptions }
	storages: StorageOption[]
	certificates: CertificateConfig[]
	tunneler: TunnelConfig
	cache: CacheConfig
	serve_limit: ServeLimitConfig
	api_rate_limit: APIRateLimitConfig
	notification: NotificationConfig
	dashboard: DashboardConfig
	github_api: GithubAPIConfig
	database: DatabaseConfig
	hijack: HijackConfig
	webdav_users: { [name: string]: WebDavUser }
	advanced: AdvancedConfig
}

export interface ClusterOptions {
	id: string
	secret: string
	byoc: boolean
	public_hosts: string[]
	server: string
	skip_signature_check: boolean
	storages: string[]
}

interface LocalStorageOption {
	type: 'local'
	cache_path: string
	compressor: string
}

interface MountStorageOption {
	type: 'mount'
	path: string
	redirect_base: string
	pre_gen_measures: boolean
}

type WebDavStorageOption = {
	type: 'webdav'
	max_conn: number
	max_upload_rate: number
	max_download_rate: number
	pre_gen_measures: boolean
	follow_redirect: boolean
	redirect_link_cache: number
	alias?: string
} & WebDavUser

export interface WebDavUser {
	endpoint?: string
	username?: string
	password?: string
}

export type StorageOption = {
	id: string
	weight: number
} & (LocalStorageOption | MountStorageOption | WebDavStorageOption)

export interface CertificateConfig {
	cert: string
	key: string
}

export interface TunnelConfig {
	enable: boolean
	tunnel_program: string
	output_regex: string
}

export type CacheConfig =
	| {
			type: 'no-cache' | 'memory'
	  }
	| {
			type: 'redis'
			network: string
			addr: string
			client_name: string
			username: string
			password: string
	  }

export interface ServeLimitConfig {
	enable: boolean
	max_conn: number
	upload_rate: number
}

export interface RateLimit {
	per_minute: number
	per_hour: number
}

export interface APIRateLimitConfig {
	anonymous: RateLimit
	logged: RateLimit
}

export interface NotificationConfig {
	enable_email: boolean
	email_smtp: string
	email_smtp_encryption: string
	email_sender: string
	email_sender_password: string
	enable_webhook: boolean
}

export interface DashboardConfig {
	enable: boolean
	username: string
	password: string
	pwa_name: string
	pwa_short_name: string
	pwa_description: string
	notification_subject: string
}

export interface GithubAPIConfig {
	update_check_interval: number
	authorization: string
}

export interface DatabaseConfig {
	driver: string
	data_source_name: string
}

export interface UserItem {
	username: string
	password: string
}

export interface HijackConfig {
	enable: boolean
	enable_local_cache: boolean
	local_cache_path: string
	require_auth: boolean
	auth_users: UserItem[]
}

export interface AdvancedConfig {
	// Unsupported
}

export async function getConfig(token: string): Promise<Config> {
	const res = await axios.get<Config>(`/api/v0/config`, {
		headers: {
			Authorization: `Bearer ${token}`,
		},
	})
	return res.data
}

export async function putConfig(token: string, config: Config): Promise<void> {
	await axios.put(`/api/v0/config`, JSON.stringify(config), {
		headers: {
			Authorization: `Bearer ${token}`,
			'Content-Type': 'application/json',
		},
	})
}

export enum ClusterStatus {
	DISCONNECTED = 0,
	CONNECTING = 1,
	DISABLED = 2,
	ENABLING = 3,
	ENABLED = 4,
}

export interface ClusterStatusRes {
	[name: string]: {
		status: ClusterStatus
		sync: boolean
	}
}

export async function getClusterStatus(token: string): Promise<ClusterStatusRes> {
	const res = await axios.get<ClusterStatusRes>(`/api/v0/cluster/status`, {
		headers: {
			Authorization: `Bearer ${token}`,
		},
	})
	return res.data
}
