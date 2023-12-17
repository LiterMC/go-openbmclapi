<script setup lang="ts">
import { onMounted, ref, computed, watch } from 'vue'
import Chart from 'primevue/chart'
import { formatNumber, formatBytes } from '@/utils'
import type { StatInstData } from '@/api/v0'

const props = defineProps<{
	width?: number | string
	height?: number | string
	max: number
	offset: number
	data: StatInstData[]
	oldData: StatInstData[]
	current: number
	formatXLabel: (index: number) => string
}>()

const maxX = props.max

const chartObj = ref()
const chartData = ref()
const chartOptions = ref()
const chartCurrentLineX = ref(-1)
var xOffset = 0

const getChartData = () => {
	const documentStyle = getComputedStyle(document.documentElement)

	const labels = Array(maxX)
	const hits = Array(maxX)
	const bytes = Array(maxX)
	const updateStat = (stats: StatInstData[], current: number) => {
		const oldStats = props.oldData
		const offset = Math.floor(current - props.offset)
		if (offset >= 0) {
			let i = 0
			for (; i + offset < stats.length; i++) {
				hits[i] = stats[i + offset].hits
				bytes[i] = stats[i + offset].bytes
			}
			for (; i < maxX; i++) {
				hits[i] = 0
				bytes[i] = 0
			}
		} else if (offset < 0) {
			let i = 0
			for (; i + offset < 0; i++) {
				let j = i + offset + oldStats.length
				hits[i] = oldStats[j].hits
				bytes[i] = oldStats[j].bytes
			}
			for (; i < maxX; i++) {
				hits[i] = stats[i + offset].hits
				bytes[i] = stats[i + offset].bytes
			}
		}
		for (let i = 0; i < maxX; i++) {
			labels[i] = props.formatXLabel(i + offset + 1)
		}
		xOffset = offset
		chartCurrentLineX.value = current - offset - 1
	}
	updateStat(props.data, props.current)
	watch(
		(): [StatInstData[], number] => [props.data, props.current],
		([stat, current]) => {
			updateStat(stat, current)
			chartObj.value.refresh()
		},
	)
	return {
		labels: labels,
		datasets: [
			{
				label: 'Hits',
				fill: true,
				borderColor: documentStyle.getPropertyValue('--blue-500'),
				yAxisID: 'y',
				tension: 0.3,
				data: hits,
			},
			{
				label: 'Bytes',
				fill: true,
				borderColor: documentStyle.getPropertyValue('--green-500'),
				yAxisID: 'y1',
				tension: 0.4,
				data: bytes,
			},
		],
	}
}

const getChartOptions = () => {
	const documentStyle = getComputedStyle(document.documentElement)
	const textColor = documentStyle.getPropertyValue('--text-color')
	const textColorSecondary = documentStyle.getPropertyValue('--text-color-secondary')
	const surfaceBorder = documentStyle.getPropertyValue('--surface-border')

	return {
		stacked: false,
		maintainAspectRatio: false,
		aspectRatio: 0.4,
		interaction: {
			mode: 'index',
			intersect: false,
		},
		plugins: {
			tooltip: {
				callbacks: {
					title: (context: any) => {
						const i = context[0].dataIndex
						return `${props.formatXLabel(xOffset + i)} ~ ${props.formatXLabel(xOffset + i + 1)}`
					},
					label: (context: any) => {
						switch (context.dataset.yAxisID) {
							case 'y':
								context.formattedValue = formatNumber(context.raw)
								break
							case 'y1':
								context.formattedValue = formatBytes(context.raw)
								break
						}
					},
				},
			},
			legend: {
				labels: {
					color: textColor,
				},
			},
			'custom-vertical-line': {
				lineX: chartCurrentLineX,
			},
		},
		scales: {
			x: {
				ticks: {
					color: textColorSecondary,
				},
				grid: {
					color: surfaceBorder,
				},
			},
			y: {
				type: 'linear',
				display: true,
				position: 'left',
				ticks: {
					color: textColorSecondary,
					callback: formatNumber,
				},
				grid: {
					color: surfaceBorder,
				},
			},
			y1: {
				type: 'linear',
				display: true,
				position: 'right',
				ticks: {
					color: textColorSecondary,
					callback: formatBytes,
				},
				grid: {
					drawOnChartArea: false,
					color: surfaceBorder,
				},
			},
		},
	}
}

onMounted(() => {
	chartData.value = getChartData()
	chartOptions.value = getChartOptions()
})
</script>

<template>
	<Chart
		ref="chartObj"
		type="line"
		:data="chartData"
		:options="chartOptions"
		:style="{ height: height, width: width }"
	/>
</template>
