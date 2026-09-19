<template>
	<view class="page">
		<view class="empty fc-card">
			<text class="empty-emoji">🌱</text>
			<text class="empty-title">动态即将上线</text>
			<text class="empty-sub">打卡后可选择分享到这里</text>
			<text class="empty-sub">优秀 = 坚持、真实、具体</text>
		</view>
	</view>
</template>
<script setup lang="ts"></script>
<style>
.page { display: flex; align-items: center; justify-content: center; min-height: 100vh; background: var(--fc-bg, #F7F7F8); padding: 24rpx; box-sizing: border-box; }
.empty { display: flex; flex-direction: column; align-items: center; padding: 72rpx 48rpx; width: 100%; box-sizing: border-box; }
.empty-emoji { font-size: 72rpx; line-height: 1; margin-bottom: 24rpx; }
.empty-title { font-size: 30rpx; font-weight: 600; color: var(--fc-text, #1F2329); }
.empty-sub { color: var(--fc-text-sub, #86909C); font-size: 26rpx; text-align: center; line-height: 1.8; margin-top: 12rpx; white-space: pre-line; }
</style>
