import type { Ref } from 'vue'
import { Chart } from 'chart.js/auto'

interface Option {
	lineX: number | Readonly<Ref<Readonly<number>>> | null
	lineWidth: number
	lineColor: string
}

Chart.register({
	id: 'custom-vertical-line',
	afterDatasetDraw: function (chart, args, { lineX, lineWidth, lineColor }: Option) {
		if (typeof lineX === 'object' && lineX) {
			lineX = lineX.value
		}
		if (typeof lineX === 'number' && lineX >= 0) {
			const ctx = chart.ctx
			const x = (lineX / chart.scales.x.max) * chart.chartArea.width + chart.chartArea.left
			const { top, bottom } = chart.scales.y

			ctx.save()
			ctx.beginPath()
			ctx.moveTo(x, top)
			ctx.lineTo(x, bottom)
			ctx.lineWidth = lineWidth
			ctx.strokeStyle = lineColor
			ctx.stroke()
			ctx.restore()
		}
	},
	defaults: {
		lineX: null,
		lineWidth: 1,
		lineColor: '#ee5522',
	} as Option,
})
