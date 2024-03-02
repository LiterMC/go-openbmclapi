
export class RingBuffer<T> {
	private readonly arr: T[]
	private start: number
	private end: number

	constructor(size: number) {
		this.arr = Array(size)
		this.start = 0
		this.end = 0
	}
}
