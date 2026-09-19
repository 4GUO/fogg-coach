<template>
	<view class="plan-page fc-safe-bottom" v-if="plan">
		<view class="head fc-card">
			<text class="wish">{{ plan.data.wish }}</text>
			<text class="note">这是实验，不是承诺。失败了 = 设计要改，不是你的问题。</text>
		</view>

		<view v-for="h in plan.data.habits" :key="h.id" class="recipe fc-card">
			<text class="title">{{ h.title }}</text>
			<view class="row"><text class="k">锚点</text><text class="v">{{ h.anchor }}</text></view>
			<view class="row"><text class="k">微行为</text><text class="v strong">{{ h.behavior }}</text></view>
			<view class="row"><text class="k">庆祝</text><text class="v">🌱 {{ h.celebration }}</text></view>
			<view class="row"><text class="k">提醒</text><text class="v">{{ h.anchor_time }}</text></view>
			<view class="prog">稳定 7 次后可升级：{{ h.progression[0].behavior }}</view>
		</view>

		<view class="coach fc-card">
			<text class="coach-label">教练寄语</text>
			<text class="coach-note">{{ plan.data.coach_note }}</text>
			<text class="review">每周{{ dayCn }}复盘一次</text>
		</view>
	</view>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { api, ensureLogin } from '../../utils/request'
import { onLoad } from '@dcloudio/uni-app'

const plan = ref<any>(null)
const days: Record<string, string> = { monday: '一', tuesday: '二', wednesday: '三', thursday: '四', friday: '五', saturday: '六', sunday: '日' }
const dayCn = ref('日')

onLoad(async (q: any) => {
	if (!(await ensureLogin())) return
	try {
		const r = await api('/plans')
		const all = r.plans || []
		plan.value = q?.id ? all.find((p: any) => p.id === q.id) : all[0]
		if (plan.value) dayCn.value = days[plan.value.data.review_day] || '日'
	} catch { }
})
</script>

<style>
.plan-page { padding: 24rpx; background: var(--fc-bg, #F7F7F8); min-height: 100vh; }
.head { margin-bottom: 24rpx; }
.wish { font-size: 36rpx; font-weight: 700; color: var(--fc-text, #1F2329); display: block; margin-bottom: 12rpx; }
.note { font-size: 24rpx; color: var(--fc-text-sub, #86909C); line-height: 1.6; }
.recipe { margin-bottom: 24rpx; }
.title { font-size: 30rpx; font-weight: 600; color: var(--fc-text, #1F2329); display: block; margin-bottom: 20rpx; }
.row { display: flex; gap: 20rpx; padding: 10rpx 0; align-items: baseline; }
.k { width: 110rpx; font-size: 24rpx; color: var(--fc-text-sub, #86909C); flex-shrink: 0; }
.v { font-size: 28rpx; color: var(--fc-text, #1F2329); }
.v.strong { font-weight: 600; }
.prog { margin-top: 16rpx; font-size: 24rpx; color: var(--fc-green, #2F9E6E); background: var(--fc-green-weak, #E6FCF5); border-radius: var(--fc-radius-sm, 16rpx); padding: 14rpx 20rpx; }
.coach-label { font-size: 24rpx; color: var(--fc-text-sub, #86909C); display: block; margin-bottom: 12rpx; }
.coach-note { font-size: 28rpx; color: var(--fc-text, #1F2329); line-height: 1.7; display: block; }
.review { font-size: 24rpx; color: var(--fc-text-sub, #86909C); margin-top: 16rpx; display: block; }
</style>
