<script setup lang="ts">
import { computed } from 'vue'
import Skeleton from 'primevue/skeleton'
import HitsChart from '@/components/HitsChart.vue'
import type { Stats, StatInstData } from '@/api/v0'
import { tr } from '@/lang'

const props = defineProps<{
	stats?: Stats | null
}>()

const stat = computed(() => {
	const stats = props.stats
	if (!stats) {
		return null
	}
	return fixStats(stats)
})

function fixStats(stats: Stats): Stats {
	stats.days = cutDays(stats.days, stats.date.year, stats.date.month)
	stats.prev.days = cutDays(stats.prev.days, stats.date.year, stats.date.month - 1)

	stats.days[stats.date.day] = stats.hours.reduce((sum, v) => ({
		hits: sum.hits + v.hits,
		bytes: sum.bytes + v.bytes,
	}))
	stats.months[stats.date.month] = stats.days.reduce((sum, v) => ({
		hits: sum.hits + v.hits,
		bytes: sum.bytes + v.bytes,
	}))
	stats.years[stats.date.year.toString()] = stats.months.reduce((sum, v) => ({
		hits: sum.hits + v.hits,
		bytes: sum.bytes + v.bytes,
	}))
	return stats
}

function formatHour(hour: number): string {
	const offset = -new Date().getTimezoneOffset()
	let min = hour * 60 + offset
	hour = Math.floor(min / 60) % 24
	min %= 60
	if (hour < 0) {
		hour += 24
	}
	if (min < 0) {
		min += 60
	}
	return `${hour}:${min.toString().padStart(2, '0')}`
}

function cutDays(days: StatInstData[], year: number, month: number): StatInstData[] {
	const dayCount = new Date(year, month, 0).getDate()
	days.length = dayCount
	return days
}

function formatDay(day: number): string {
	if (!stat.value) {
		return ''
	}
	const date = new Date(Date.UTC(stat.value.date.year, stat.value.date.month, day))
	return `${date.getMonth() + 1}-${date.getDate()}`
}

function formatMonth(month: number): string {
	if (!stat.value) {
		return ''
	}
	const date = new Date(Date.UTC(stat.value.date.year, month + 1, 1))
	return `${date.getFullYear()}-${(date.getMonth() + 1).toString().padStart(2, '0')}`
}

function getDaysInMonth(): number {
	const date = new Date()
	const days = new Date(date.getFullYear(), date.getMonth() + 1, 0).getDate()
	return date.getDate() / days
}
</script>
<template>
	<div class="hits-chart-box">
		<div class="chart-card">
			<h3>{{ tr('title.day') }}</h3>
			<HitsChart
				v-if="stat"
				class="hits-chart"
				:max="25"
				:offset="23"
				:data="stat.hours"
				:oldData="stat.prev.hours"
				:current="stat.date.hour + new Date().getMinutes() / 60"
				:formatXLabel="formatHour"
			/>
			<Skeleton v-else width="" height="" class="hits-chart" />
		</div>
		<div class="chart-card">
			<h3>{{ tr('title.month') }}</h3>
			<HitsChart
				v-if="stat"
				class="hits-chart"
				:max="31"
				:offset="29"
				:data="stat.days"
				:oldData="stat.prev.days"
				:current="stat.date.day + new Date().getUTCHours() / 24"
				:formatXLabel="formatDay"
			/>
			<Skeleton v-else width="" height="" class="hits-chart" />
		</div>
		<div class="chart-card">
			<h3>{{ tr('title.year') }}</h3>
			<HitsChart
				v-if="stat"
				class="hits-chart"
				:max="13"
				:offset="11"
				:data="stat.months"
				:oldData="stat.prev.months"
				:current="stat.date.month + getDaysInMonth()"
				:formatXLabel="formatMonth"
			/>
			<Skeleton v-else width="" height="" class="hits-chart" />
		</div>
		<!-- TODO: show yearly chart -->
	</div>
</template>
<style scoped>
.hits-chart-box {
	grid-area: b;
}

.chart-card {
	margin-bottom: 1rem;
}

.hits-chart {
	max-width: 100%;
	width: 45rem;
	height: 13rem;
	user-select: none;
}

@media (max-width: 60rem) {
	.hits-chart {
		width: 100%;
	}
}
</style>
