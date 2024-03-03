export class RingBuffer<T> extends Array<T> {
	private start: number
	private end: number

	private constructor(size: number) {
		super(size + 1)
		this.start = 0
		this.end = 0
	}

	get realLength(): number {
		return Reflect.get(this, 'length')
	}

	get size(): number {
		return this.realLength - 1
	}

	get avaliable(): number {
		if (this.start <= this.end) {
			return this.end - this.start
		}
		return this.length - this.start + this.end
	}

	at(i: number): T {
		return this[(this.start + i) % this.length]
	}

	set(i: number, v: T): void {
		this[(this.start + i) % this.length] = v
	}

	push(...items: T[]): number {
		if (items.length >= this.size) {
			this.splice(0, this.size, ...items.slice(-this.size))
			this.start = 0
			this.end = this.size
			return this.avaliable
		}
		const adding = items.length
		const right = this.realLength - this.end
		this.splice(this.end, right, ...items.slice(0, right))
		if (adding > right) {
			this.splice(0, adding - right, ...items.slice(right))
		}
		const oldend = this.end
		this.end = (oldend + adding) % this.realLength
		if (this.start <= this.end && this.end < oldend || oldend < this.start && this.start <= this.end) {
			this.start = (this.end + 1) % this.realLength
		}
		return this.avaliable
	}

	*keys(): IterableIterator<number> {
		const start = this.start
		const end = this.end
		const length = this.realLength
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
		for(let i of this.keys()){
			yield Reflect.get(this, i)
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
				}
				return Reflect.get(target, key)
			},
			set(target: RingBuffer<T>, key: string | symbol, value: any): boolean {
				if (typeof key === 'string' && /^[1-9][0-9]*$/.test(key)) {
					target.set(parseInt(key, 10), value)
					return true
				}
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
