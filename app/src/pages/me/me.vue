<template>
	<view class="me">
		<view class="card">
			<text class="label">用户</text>
			<text class="val">{{ user?.userId || '未登录' }}</text>
		</view>
		<view class="card muted">
			<text class="tip">⚠️ 本产品是习惯养成工具，不提供医疗或心理健康诊断服务。如需专业帮助，请咨询医生。</text>
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
.me { padding: 24rpx; min-height: 100vh; background: #f7f8fa; }
.card { background: #fff; border-radius: 20rpx; padding: 28rpx; margin-bottom: 20rpx; display: flex; flex-direction: column; gap: 8rpx; }
.label { font-size: 24rpx; color: #9ca3af; }
.val { font-size: 28rpx; color: #1f2937; font-family: monospace; }
.muted { background: #fffbeb; }
.tip { font-size: 24rpx; color: #92400e; line-height: 1.7; }
.logout { background: #fff; color: #ef4444; font-size: 26rpx; border-radius: 12rpx; margin-top: 40rpx; }
</style>
