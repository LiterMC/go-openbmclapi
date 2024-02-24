
export {
	type LogMsg,
	LogIO,
}

interface BasicMsg {
	type: string
}

interface ErrorMsg {
	type: "error"
	message: string
}

interface LogMsg {
	type: "log"
	time: number
	lvl: string
	log: string
}

class LogIO {
	private ws: WebSocket | null = null
	private logListener: ((msg: LogMsg) => void)[] = []
	private closeListener: (() => void)[] = []

	constructor(ws: WebSocket){
		this.setWs(ws)
	}

	private setWs(ws: WebSocket): void {
		ws.addEventListener('close', this.onClose)
		ws.addEventListener('message', (msg) => {
			const res = JSON.parse(msg.data) as BasicMsg
			this.onMessage(res)
		})
		this.ws = ws
	}

	close(): void {
		if(this.ws){
			this.ws.close()
			this.ws = null
		}
	}

	private onClose(event: CloseEvent): void {
		for(const l of this.closeListener){
			l()
		}
	}

	private onMessage(msg: BasicMsg): void {
		switch(msg.type){
		case 'error':
			break
		case 'log':
			this.onLog(msg as LogMsg)
			break
		}
	}

	private onLog(msg: LogMsg): void {
		for(const l of this.logListener){
			l(msg)
		}
	}

	addLogListener(l: (msg: LogMsg) => void): void {
		this.logListener.push(l)
	}

	addCloseListener(l: () => void): void {
		this.closeListener.push(l)
	}

	static async dial(token: string): Promise<LogIO> {
		const wsTarget = `${httpToWs(window.location.protocol)}//${window.location.host}/api/v0/log.io`
		const ws = new WebSocket(wsTarget)

		var connTimeout: ReturnType<typeof setTimeout>
		await new Promise<void>((resolve, reject) => {
			connTimeout = setTimeout(() => {
				reject('WebSocket dial timeout')
				ws.close()
			}, 1000 * 15)
			ws.addEventListener('error', reject)
			ws.addEventListener('open', () => {
				ws.removeEventListener('error', reject)
				resolve()
			})
		}).finally(() => clearTimeout(connTimeout))

		var after: (() => void)
		await new Promise<void>((resolve, reject) => {
			const listener = (msg: MessageEvent) => {
				try{
					const res = JSON.parse(msg.data) as BasicMsg
					if(res.type === 'error'){
						reject((res as ErrorMsg).message)
					}else if(res.type === 'ready'){
						resolve()
					}
				}catch(err){
					reject(err)
				}
			}
			ws.addEventListener('message', listener)
			after = () => ws.removeEventListener('message', listener)
			ws.send(JSON.stringify({
				token: token,
			}))
		}).finally(() => after())

		return new LogIO(ws)
	}
}

function httpToWs(protocol: string): string {
	return protocol == 'http:' ? 'ws:' : 'wss:'
}
