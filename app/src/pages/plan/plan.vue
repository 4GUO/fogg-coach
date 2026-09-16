<template>
	<view class="plan-page" v-if="plan">
		<view class="head">
			<text class="wish">{{ plan.data.wish }}</text>
			<text class="note">这是实验，不是承诺。失败了 = 设计要改，不是你的问题。</text>
		</view>

		<view v-for="h in plan.data.habits" :key="h.id" class="recipe">
			<text class="title">{{ h.title }}</text>
			<view class="row"><text class="k">锚点</text><text class="v">{{ h.anchor }}</text></view>
			<view class="row"><text class="k">微行为</text><text class="v strong">{{ h.behavior }}</text></view>
			<view class="row"><text class="k">庆祝</text><text class="v">{{ h.celebration }}</text></view>
			<view class="row"><text class="k">提醒</text><text class="v">{{ h.anchor_time }}</text></view>
			<view class="prog">🌱 稳定 7 次后可升级：{{ h.progression[0].behavior }}</view>
		</view>

		<view class="coach">
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
.plan-page { padding: 24rpx; background: #f7f8fa; min-height: 100vh; }
.head { background: #fff; border-radius: 20rpx; padding: 32rpx; margin-bottom: 24rpx; }
.wish { font-size: 36rpx; font-weight: 700; color: #1f2937; display: block; margin-bottom: 12rpx; }
.note { font-size: 24rpx; color: #9ca3af; }
.recipe { background: #fff; border-radius: 20rpx; padding: 28rpx; margin-bottom: 24rpx; }
.title { font-size: 30rpx; font-weight: 600; color: #1f2937; display: block; margin-bottom: 20rpx; }
.row { display: flex; gap: 20rpx; padding: 10rpx 0; }
.k { width: 110rpx; font-size: 24rpx; color: #9ca3af; flex-shrink: 0; }
.v { font-size: 28rpx; color: #374151; }
.v.strong { font-weight: 600; color: #1f2937; }
.prog { margin-top: 16rpx; font-size: 24rpx; color: #2F9E6E; background: #e6fcf5; border-radius: 12rpx; padding: 14rpx 20rpx; }
.coach { background: #fff; border-radius: 20rpx; padding: 28rpx; }
.coach-label { font-size: 24rpx; color: #9ca3af; display: block; margin-bottom: 12rpx; }
.coach-note { font-size: 28rpx; color: #374151; line-height: 1.7; display: block; }
.review { font-size: 24rpx; color: #6b7280; margin-top: 16rpx; display: block; }
</style>
