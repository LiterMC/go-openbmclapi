<script setup lang="ts">
import { onMounted, ref, watch, computed } from 'vue'
import Chart from 'primevue/chart'
import { formatNumber } from '@/utils'
import { tr } from '@/lang'

const props = defineProps<{
	max: number
	data: { [ua: string]: number }
}>()

const data = computed(() => Object.entries(props.data).
	map(([ua, count]) => ({ ua: ua, count: count })).
	sort((a, b) => b.count - a.count).
	slice(0, 7))

const chartObj = ref()
const chartData = ref()
const chartOptions = ref()

const getChartData = () => {
	const documentStyle = getComputedStyle(document.documentElement)

	const labels = data.value.map(({ua}) => ua)
	const counts = data.value.map(({count}) => count)
	watch(data, (data) => {
		const length = data.length
		labels.length = length
		counts.length = length
		for(let i = 0; i < length; i++){
			({
				ua: labels[i],
				count: counts[i],
			} = data[i])
		}
		chartObj.value.refresh()
	})

	const colors = [
		documentStyle.getPropertyValue('--red-500'),
		documentStyle.getPropertyValue('--orange-500'),
		documentStyle.getPropertyValue('--yellow-500'),
		documentStyle.getPropertyValue('--green-500'),
		documentStyle.getPropertyValue('--cyan-500'),
		documentStyle.getPropertyValue('--blue-500'),
		documentStyle.getPropertyValue('--purple-500'),
	]
	return {
		labels: labels,
		datasets: [
			{
				label: computed(() => tr('title.hits')),
				data: counts,
				backgroundColor: colors.map((rgb) => rgb + '33'),
				borderColor: colors,
				borderWidth: 1,
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
		indexAxis: 'y',
		stacked: false,
		maintainAspectRatio: false,
		interaction: {
			mode: 'index',
			axis: 'y',
			intersect: false,
		},
		animation: {},
		plugins: {
			tooltip: {
				callbacks: {
					label: (context: any) => {
						switch (context.dataset.yAxisID) {
							case 'y':
								context.formattedValue = formatNumber(context.raw)
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
		},
		scales: {
			y: {
				ticks: {
					color: textColorSecondary,
				},
				grid: {
					color: surfaceBorder,
				},
			},
			x: {
				type: 'linear',
				display: true,
				beginAtZero: true,
				position: 'left',
				ticks: {
					color: textColorSecondary,
					callback: formatNumber,
				},
				grid: {
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
		type="bar"
		:data="chartData"
		:options="chartOptions"
	/>
</template>