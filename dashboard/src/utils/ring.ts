export class RingBuffer<T> extends Array<T> {
	private start: number
	private end: number

	private constructor(size: number) {
		super(size + 1)
		this.start = 0
		this.end = 0
	}

	get _realLength(): number {
		return this.length
	}

	_realGet(i: number): T {
		return this[i]
	}

	_realSet(i: number, v: T): void {
		this[i] = v
	}

	_realReplace(start: number, items: T[]): void {
		for (let i = 0; i < items.length; i++) {
			this._realSet(start + i, items[i])
		}
		// this.splice(start, items.length, ...items)
	}

	get size(): number {
		return this._realLength - 1
	}

	get avaliable(): number {
		if (this.start <= this.end) {
			return this.end - this.start
		}
		return this.length - this.start + this.end
	}

	at(i: number): T {
		return this._realGet((this.start + i) % this.length)
	}

	set(i: number, v: T): void {
		this._realSet((this.start + i) % this.length, v)
	}

	push(...items: T[]): number {
		if (items.length >= this.size) {
			this._realReplace(0, items.slice(-this.size))
			this.start = 0
			this.end = this.size
			return this.avaliable
		}
		const adding = items.length
		const right = this._realLength - this.end
		if (adding > right) {
			this._realReplace(this.end, items.slice(0, right))
			this._realReplace(0, items.slice(right))
		} else {
			this._realReplace(this.end, items)
		}
		const oldend = this.end
		this.end = (oldend + adding) % this._realLength
		if (
			(this.start <= this.end && this.end < oldend) ||
			(oldend < this.start && this.start <= this.end)
		) {
			this.start = (this.end + 1) % this._realLength
		}
		return this.avaliable
	}

	*keys(): IterableIterator<number> {
		const start = this.start
		const end = this.end
		const length = this._realLength
		if (start <= end) {
			for (let i = start; i < end; i++) {
				yield i
			}
		} else {
			for (let i = start; i < length; i++) {
				yield i
			}
			for (let i = 0; i < end; i++) {
				yield i
			}
		}
	}

	*values(): IterableIterator<T> {
		for (let i of this.keys()) {
			yield this._realGet(i)
		}
	}

	[Symbol.iterator](): IterableIterator<T> {
		return this.values()
	}

	static create<T>(size: number): T[] {
		const ring = new this<T>(size)
		return new Proxy(ring, {
			get(target: RingBuffer<T>, key: string | symbol): any {
				if (typeof key === 'string') {
					const index = parseInt(key, 10)
					if (!isNaN(index)) {
						return target.at(index)
					}
				}
				switch (key) {
					case 'length':
						return target.avaliable
					case '_realLength':
						return target.length
					case '_realGet':
						// case '_realSet':
						// case '_realReplace':
						return Reflect.get(target, key).bind(target)
				}
				return Reflect.get(target, key)
			},
			set(
				target: RingBuffer<T>,
				key: string | symbol,
				value: any,
				recvier: RingBuffer<T>,
			): boolean {
				// if (typeof key === 'string' && /^[1-9][0-9]*$/.test(key)) {
				// 	recvier.set(parseInt(key, 10), value)
				// 	return true
				// }
				return Reflect.set(target, key, value)
			},
			ownKeys(target: RingBuffer<T>): (string | symbol)[] {
				const keys: (string | symbol)[] = Array.of(...target.keys()).map((n) => n.toString())
				for (let k of Reflect.ownKeys(target)) {
					if (typeof k !== 'string' || !/^[1-9][0-9]*$/.test(k)) {
						keys.push(k)
					}
				}
				return keys
			},
		})
	}
}
