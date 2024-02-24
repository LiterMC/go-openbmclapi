const CODERE = /^([a-zA-Z]{2})(?:[_-]([a-zA-Z]{2}))?$/

export class Lang {
	readonly name: string
	readonly area?: string

	constructor(code: string) {
		const group = CODERE.exec(code.toLowerCase())
		if (!group) {
			throw `"${code}" is not a valid language code`
		}
		;[, this.name, this.area] = group
	}

	toString(): string {
		return this.area ? this.name + '-' + this.area.toUpperCase() : this.name
	}

	equals(code: unknown): boolean {
		if (!code) {
			return false
		}
		if (typeof code === 'string') {
			const group = CODERE.exec(code.toLowerCase())
			if (!group) {
				return false
			}
			return this.name == group[1] && this.area == group[2]
		}
		if (code instanceof Lang) {
			return this.name == code.name && this.area == code.area
		}
		return false
	}

	match(code: unknown): boolean {
		if (!code) {
			return false
		}
		if (typeof code === 'string') {
			const group = CODERE.exec(code.toLowerCase())
			if (!group) {
				return false
			}
			return this.name == group[1] && (!this.area || this.area == group[2])
		}
		if (code instanceof Lang) {
			return this.name == code.name && (!this.area || this.area == code.area)
		}
		return false
	}
}
