<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRequest } from 'vue-request'
import axios from 'axios'
import { formatNumber, formatBytes } from '@/utils'
import HitsChart from '@/components/HitsChart.vue'

const { data, loading } = useRequest(async () => (await axios.get('/api/status.json')).data, {
	pollingInterval: 5000,
})

// const days = new Date(stat.date.year, stat.date.month + 1, 0).getDate();
</script>

<template>
	<main>
		<h4>Daily</h4>
		<HitsChart
			v-if="data"
			:max="24"
			:data="data.stat.hours"
			:oldData="data.stat.prev.hours"
			:current="data.stat.date.hour + new Date().getMinutes() / 60"
			:formatXLabel="(i) => `${i + 1}:00`"
		/>
	</main>
</template>
