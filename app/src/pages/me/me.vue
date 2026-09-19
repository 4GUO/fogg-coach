<template>
	<view class="me fc-safe-bottom">
		<view class="card fc-card">
			<text class="label fc-sub">用户</text>
			<text class="val">{{ user?.userId || '未登录' }}</text>
		</view>
		<view class="card notice fc-card">
			<text class="tip">本产品是习惯养成工具，不提供医疗或心理健康诊断服务。如需专业帮助，请咨询医生。</text>
		</view>
		<button class="logout" @click="logout">清除登录（开发）</button>
	</view>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { getToken } from '../../utils/request'
const user = ref<any>(uni.getStorageSync('fc_user') || null)
if (!user.value && getToken()) user.value = { userId: '已登录' }
function logout() {
	uni.removeStorageSync('fc_token')
	uni.removeStorageSync('fc_user')
	uni.showToast({ title: '已清除', icon: 'none' })
}
</script>

<style>
.me { padding: 24rpx; min-height: 100vh; background: var(--fc-bg, #F7F7F8); }
.card { margin-bottom: 20rpx; display: flex; flex-direction: column; gap: 8rpx; }
.label { font-size: 24rpx; color: var(--fc-text-sub, #86909C); }
.val { font-size: 28rpx; color: var(--fc-text, #1F2329); word-break: break-all; }
.notice { background: var(--fc-bg, #F7F7F8); border-left: 6rpx solid var(--fc-primary, #4C6EF5); }
.tip { font-size: 24rpx; color: var(--fc-text-sub, #86909C); line-height: 1.7; }
.logout { background: var(--fc-card, #fff); color: #E5484D; font-size: 26rpx; border-radius: var(--fc-radius-sm, 16rpx); margin-top: 40rpx; border: 1rpx solid var(--fc-border, #E5E6EB); }
</style>
