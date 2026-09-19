<script setup lang="ts">
import { onLaunch, onShow, onHide } from "@dcloudio/uni-app";
onLaunch(() => {
  console.log("App Launch");
});
onShow(() => {
  console.log("App Show");
});
onHide(() => {
  console.log("App Hide");
});
</script>
<style>
/**
 * 全局设计令牌（docs/design-style.md）：
 * 底色 #F7F7F8 / 卡片白 / 主强调 #4C6EF5（定稿）/ 辅助绿 #2F9E6E
 * 文字 #1F2329 主、#86909C 次；圆角 8-12px（16-24rpx）
 * 页面统一引用 var(--fc-*)，禁止再各自硬编码
 */
page {
	--fc-primary: #4C6EF5;
	--fc-primary-weak: #EEF2FF;
	--fc-primary-disabled: #C7D2FE;
	--fc-green: #2F9E6E;
	--fc-green-weak: #E6FCF5;
	--fc-bg: #F7F7F8;
	--fc-card: #FFFFFF;
	--fc-text: #1F2329;
	--fc-text-sub: #86909C;
	--fc-border: #E5E6EB;
	--fc-radius: 24rpx;   /* 卡片圆角 12px */
	--fc-radius-sm: 16rpx; /* 控件圆角 8px */
	background: var(--fc-bg);
	color: var(--fc-text);
	font-size: 28rpx;
}

/* 通用卡片：白底 + 细边框（轻投影可选，克制优先） */
.fc-card {
	background: var(--fc-card);
	border-radius: var(--fc-radius);
	border: 1rpx solid var(--fc-border);
	padding: 28rpx;
}

/* 通用按钮 */
.fc-btn-primary {
	background: var(--fc-primary);
	color: #fff;
	border-radius: var(--fc-radius-sm);
	font-size: 28rpx;
}
.fc-btn-ghost {
	background: var(--fc-card);
	color: var(--fc-text-sub);
	border: 1rpx solid var(--fc-border);
	border-radius: var(--fc-radius-sm);
	font-size: 26rpx;
}

/* iOS 底部安全区 */
.fc-safe-bottom { padding-bottom: calc(24rpx + env(safe-area-inset-bottom)); }

/* 次要说明文字 */
.fc-sub { color: var(--fc-text-sub); font-size: 24rpx; }
</style>
