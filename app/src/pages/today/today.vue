<template>
	<view class="today">
		<view v-if="!plans.length" class="empty">
			<text class="empty-emoji">🌱</text>
			<text class="empty-title">还没有进行中的计划</text>
			<text class="empty-sub">和教练聊 5 分钟，拿到你的第一个微习惯实验</text>
			<button class="primary" @click="goChat">开始对话</button>
		</view>

		<view v-else>
			<view v-for="p in plans" :key="p.id" class="plan-card" @click="openPlan(p)">
				<view class="plan-head">
					<text class="wish">{{ p.data.wish }}</text>
					<text class="status">{{ statusText(p.status) }}</text>
				</view>
				<view v-for="h in p.data.habits" :key="h.id" class="habit">
					<text class="anchor">⚓ {{ h.anchor }}</text>
					<text class="behavior">{{ h.behavior }}</text>
					<text class="celebrate">🎉 {{ h.celebration }}</text>
				</view>
			</view>
			<view class="hint">打卡功能即将上线（M3）</view>
		</view>
	</view>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { api, ensureLogin } from '../../utils/request'

interface Habit { id: string; title: string; anchor: string; behavior: string; celebration: string; anchor_time: string }
interface Plan { id: string; status: string; data: { wish: string; habits: Habit[]; coach_note: string } }

const plans = ref<Plan[]>([])

async function load() {
	if (!(await ensureLogin())) return
	try {
		const r = await api('/plans')
		plans.value = (r.plans || []).filter((p: any) => p.status === 'active')
	} catch { /* 静默 */ }
}
// onShow 每次进入刷新（从 chat 生成计划回来能立即看到）
import { onShow } from '@dcloudio/uni-app'
onShow(load)

function goChat() { uni.switchTab({ url: '/pages/chat/chat' }) }
function openPlan(p: Plan) { uni.navigateTo({ url: '/pages/plan/plan?id=' + p.id }) }
function statusText(s: string) { return s === 'active' ? '进行中' : s }
</script>

<style>
.today { padding: 24rpx; min-height: 100vh; background: #f7f8fa; }
.empty { display: flex; flex-direction: column; align-items: center; padding-top: 220rpx; gap: 16rpx; }
.empty-emoji { font-size: 96rpx; }
.empty-title { font-size: 34rpx; font-weight: 600; color: #1f2937; }
.empty-sub { font-size: 26rpx; color: #6b7280; }
.primary { margin-top: 40rpx; background: #4C6EF5; color: #fff; font-size: 30rpx; border-radius: 999rpx; padding: 0 80rpx; }
.plan-card { background: #fff; border-radius: 20rpx; padding: 28rpx; margin-bottom: 24rpx; }
.plan-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20rpx; }
.wish { font-size: 32rpx; font-weight: 600; color: #1f2937; }
.status { font-size: 22rpx; color: #2F9E6E; background: #e6fcf5; padding: 4rpx 16rpx; border-radius: 999rpx; }
.habit { padding: 20rpx; background: #f9fafb; border-radius: 14rpx; margin-bottom: 14rpx; display: flex; flex-direction: column; gap: 8rpx; }
.anchor { font-size: 24rpx; color: #6b7280; }
.behavior { font-size: 30rpx; color: #1f2937; font-weight: 500; }
.celebrate { font-size: 24rpx; color: #9ca3af; }
.hint { text-align: center; color: #c0c4cc; font-size: 24rpx; margin-top: 40rpx; }
</style>
