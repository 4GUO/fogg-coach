<template>
	<view class="today">
		<view v-if="!plans.length" class="empty fc-card">
			<text class="empty-emoji">🌱</text>
			<text class="empty-title">还没有进行中的计划</text>
			<text class="empty-sub">和教练聊 5 分钟，拿到你的第一个微习惯实验</text>
			<button class="primary" @click="goChat">开始对话</button>
		</view>

		<view v-else class="fc-safe-bottom">
			<view v-for="p in plans" :key="p.id" class="plan-card fc-card" @click="openPlan(p)">
				<view class="plan-head">
					<text class="wish">{{ p.data.wish }}</text>
					<text class="status">{{ statusText(p.status) }}</text>
				</view>
				<view v-for="h in p.data.habits" :key="h.id" class="habit">
					<view class="habit-row"><text class="k">锚点</text><text class="anchor">{{ h.anchor }}</text></view>
					<view class="habit-row"><text class="k">微行为</text><text class="behavior">{{ h.behavior }}</text></view>
					<view class="habit-row"><text class="k">庆祝</text><text class="celebrate">🌱 {{ h.celebration }}</text></view>
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
.today { padding: 24rpx; min-height: 100vh; background: var(--fc-bg, #F7F7F8); }

/* 空态引导卡 */
.empty { display: flex; flex-direction: column; align-items: center; padding: 96rpx 48rpx; margin-top: 120rpx; }
.empty-emoji { font-size: 88rpx; line-height: 1; margin-bottom: 32rpx; }
.empty-title { font-size: 34rpx; font-weight: 600; color: var(--fc-text, #1F2329); }
.empty-sub { font-size: 26rpx; color: var(--fc-text-sub, #86909C); margin-top: 12rpx; line-height: 1.6; text-align: center; }
.primary { margin-top: 48rpx; background: var(--fc-primary, #4C6EF5); color: #fff; font-size: 30rpx; border-radius: 999rpx; padding: 0 80rpx; }

/* 配方卡 */
.plan-card { margin-bottom: 24rpx; }
.plan-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20rpx; }
.wish { font-size: 32rpx; font-weight: 600; color: var(--fc-text, #1F2329); }
.status { font-size: 22rpx; color: var(--fc-green, #2F9E6E); background: var(--fc-green-weak, #E6FCF5); padding: 4rpx 16rpx; border-radius: 999rpx; flex-shrink: 0; margin-left: 16rpx; }
.habit { padding: 20rpx 24rpx; background: var(--fc-bg, #F7F7F8); border-radius: var(--fc-radius-sm, 16rpx); margin-bottom: 14rpx; display: flex; flex-direction: column; gap: 8rpx; }
.habit:last-child { margin-bottom: 0; }
.habit-row { display: flex; gap: 16rpx; align-items: baseline; }
.k { width: 90rpx; font-size: 24rpx; color: var(--fc-text-sub, #86909C); flex-shrink: 0; }
.anchor { font-size: 26rpx; color: var(--fc-text-sub, #86909C); }
.behavior { font-size: 30rpx; color: var(--fc-text, #1F2329); font-weight: 500; }
.celebrate { font-size: 24rpx; color: var(--fc-text-sub, #86909C); }
.hint { text-align: center; color: var(--fc-text-sub, #86909C); opacity: .6; font-size: 24rpx; margin-top: 40rpx; }
</style>
