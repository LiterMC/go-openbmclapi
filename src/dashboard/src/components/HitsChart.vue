<script setup lang="ts">
import { onMounted, ref, computed, watch } from 'vue'
import Chart from 'primevue/chart'
import { formatNumber, formatBytes } from '@/utils'

const props = defineProps<{
	max: number
	data: {
		hits: number
		bytes: number
	}[]
	oldData: {
		hits: number
		bytes: number
	}[]
	current: number
	formatXLabel: (index: number) => string
}>()

const maxX = props.max

const chartObj = ref()
const chartData = ref()
const chartOptions = ref()

const getChartData = () => {
	const documentStyle = getComputedStyle(document.documentElement)

	const labels = []
	for (let i = 0; i < maxX; i++) {
		labels.push(`${i + 1}`)
	}
	const hits = Array(maxX).fill(0)
	const bytes = Array(maxX).fill(0)
	for (let i = 0; i < maxX; i++) {
		hits[i] = props.data[i].hits
		bytes[i] = props.data[i].bytes
	}
	watch(
		() => props.data,
		(stat) => {
			for (let i = 0; i < maxX; i++) {
				hits[i] = stat[i].hits
				bytes[i] = stat[i].bytes
			}
			chartObj.value.refresh()
		},
	)
	watch(
		() => props.current,
		(current) => {
			chartObj.value.options.plugins['custom-vertical-line'].lineX = current
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
						return `${props.formatXLabel(i)} ~ ${props.formatXLabel(i + 1)}`
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
				lineX: props.current,
			},
		},
		scales: {
			x: {
				ticks: {
					color: textColorSecondary,
					callback: props.formatXLabel,
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
		style="height: 16rem"
	/>
</template>
