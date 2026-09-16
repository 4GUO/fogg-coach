<template>
	<view class="chat-page">
		<!-- 阶段进度条（诊断期动力装置） -->
		<view class="stage-bar" v-if="stageIdx >= 0 && stageIdx < 7">
			<view v-for="(s, i) in stages" :key="s" class="dot" :class="{ done: i < stageIdx, cur: i === stageIdx }">{{ i + 1 }}</view>
			<text class="stage-label">{{ stageNames[stageIdx] }}</text>
		</view>
		<view class="stage-bar exec" v-else-if="stage === 'S7'">
			<text class="stage-label">教练正在整理你的计划…</text>
		</view>

		<!-- 消息列表 -->
		<scroll-view class="msgs" scroll-y :scroll-top="scrollTop" :scroll-with-animation="true">
			<view v-for="(m, i) in messages" :key="i" class="msg" :class="m.role">
				<view class="bubble">{{ m.content }}</view>
				<!-- chips -->
				<view v-if="m.chips && m.chips.length && i === messages.length - 1 && !busy && stage !== 'S4'" class="chips">
					<button v-for="c in m.chips" :key="c" class="chip" size="mini" @click="send(c)">{{ c }}</button>
				</view>
			</view>
			<!-- S4 结构化选择卡（绕过 LLM，按钮事件） -->
			<view v-if="stage === 'S4' && candidates.length && !busy" class="select-card">
				<text class="card-title">勾选你想先做的 1-3 个（30 秒内能完成的）</text>
				<view class="opts">
					<button v-for="(opt, oi) in candidates" :key="opt" class="opt" size="mini"
						:class="{ on: picked.includes(oi) }" @click="togglePick(oi)">{{ oi + 1 }}. {{ opt }}</button>
				</view>
				<button class="primary" :disabled="!picked.length" @click="confirmPick">就选这些</button>
			</view>
			<!-- S6 确认卡 -->
			<view v-if="stage === 'S6' && !busy" class="select-card">
				<text class="card-title">计划准备好了，这是你的实验，不是承诺 ✨</text>
				<button class="primary" @click="doConfirm">确认，开始我的计划</button>
				<button class="ghost" @click="doRevise">再改改</button>
			</view>
		</scroll-view>

		<!-- 输入区 -->
		<view class="input-bar">
			<input v-model="draft" class="input" :disabled="busy" confirm-type="send"
				placeholder="说说你的想法…" @confirm="send(draft)" />
			<button class="send" :disabled="busy || !draft.trim()" @click="send(draft)">发送</button>
		</view>
	</view>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { api, ensureLogin } from '../../utils/request'
import { sendChat } from '../../composables/useChatStream'

const stages = ['S1', 'S2', 'S3', 'S4', 'S5', 'S6', 'S7']
const stageNames = ['探索愿望', '了解现状', '头脑风暴', '挑选行为', '设计配方', '确认计划', '生成计划']

interface Msg { role: string; content: string; chips?: string[] }
const messages = ref<Msg[]>([])
const draft = ref('')
const stage = ref('S1')
const sessionId = ref<string | null>(null)
const candidates = ref<string[]>([])
const picked = ref<number[]>([])
const busy = ref(false)
const scrollTop = ref(0)

const stageIdx = computed(() => stages.indexOf(stage.value))

onMounted(restore)

async function restore() {
	if (!(await ensureLogin())) { messages.value = [{ role: 'assistant', content: '登录失败，请稍后再试' }]; return }
	try {
		const r = await api('/session/active')
		sessionId.value = r.session.id
		stage.value = r.session.stage
		candidates.value = (r.context && r.context.candidates) || []
		const hist: Msg[] = (r.messages || [])
			.filter((m: any) => m.role === 'user' || m.role === 'assistant')
			.map((m: any) => {
				let chips: string[] | undefined
				try { const meta = JSON.parse(m.meta || '{}'); if (meta.quick_replies) chips = meta.quick_replies } catch { }
				return { role: m.role, content: m.content, chips }
			})
		messages.value = hist.length ? hist : []
		if (!hist.length) {
			messages.value = [{ role: 'assistant', content: '你好呀，我是小福，你的行为设计教练 🌱\n最近你希望生活里有什么不一样？' }]
		}
		scrollBottom()
	} catch {
		// 404 = 无进行中会话，开新对话
		messages.value = [{ role: 'assistant', content: '你好呀，我是小福，你的行为设计教练 🌱\n最近你希望生活里有什么不一样？' }]
	}
}

async function send(text: string) {
	const content = (text || '').trim()
	if (!content || busy.value) return
	draft.value = ''
	messages.value.push({ role: 'user', content })
	await stream({ sessionId: sessionId.value, content })
}

async function stream(payload: any) {
	busy.value = true
	const ai = { role: 'assistant', content: '' }
	messages.value.push(ai)
	scrollBottom()
	try {
		const evs = await sendChat(payload, {
			onDelta: (t) => { ai.content += t; scrollBottom() },
		})
		for (const e of evs) {
			if (e.sessionId) sessionId.value = e.sessionId
			if (e.event === 'quick_replies' && e.items) ai.chips = e.items
			if (e.event === 'stage_done') stage.value = e.next || stage.value
			if (e.stage) stage.value = e.stage
			if (e.candidates) candidates.value = e.candidates
			if (e.error) { ai.content = ai.content || ('⚠️ ' + (e.message || '出错了')) }
			if (e.forcePlan) await genPlan()
		}
		if (stage.value === 'S7') await genPlan()
	} catch (err: any) {
		ai.content = ai.content || ('⚠️ ' + (err?.message || '网络开小差了，再试一次'))
	} finally {
		busy.value = false
		scrollBottom()
	}
}

function togglePick(i: number) {
	const at = picked.value.indexOf(i)
	if (at >= 0) picked.value.splice(at, 1)
	else if (picked.value.length < 3) picked.value.push(i)
}
async function confirmPick() {
	const options = picked.value.map((i) => candidates.value[i])
	messages.value.push({ role: 'user', content: '我选：' + options.join('、') })
	await stream({ sessionId: sessionId.value, action: { type: 'select', options } })
}
async function doConfirm() {
	messages.value.push({ role: 'user', content: '确认，就按这个计划来' })
	await stream({ sessionId: sessionId.value, action: { type: 'confirm' } })
}
async function doRevise() {
	messages.value.push({ role: 'user', content: '有些地方想再调整一下' })
	await stream({ sessionId: sessionId.value, action: { type: 'revise' } })
}

async function genPlan() {
	try {
		const r = await api('/plan/generate', { sessionId: sessionId.value })
		messages.value.push({ role: 'assistant', content: '你的计划生成好了 🎉\n到「今日」页看看吧，从今晚就可以开始第一个小实验。' })
		stage.value = 'active'
		uni.showToast({ title: '计划已生成', icon: 'success' })
	} catch (err: any) {
		messages.value.push({ role: 'assistant', content: '⚠️ ' + (err?.message || '计划生成失败，稍后再试') })
		if (err?.code === 422) stage.value = 'S6'
	}
}

function scrollBottom() {
	setTimeout(() => { scrollTop.value = scrollTop.value === 99998 ? 99999 : 99998 }, 50)
}
</script>

<style>
.chat-page { display: flex; flex-direction: column; height: 100vh; background: #f7f8fa; }
.stage-bar { display: flex; align-items: center; gap: 10rpx; padding: 16rpx 24rpx; background: #fff; border-bottom: 1rpx solid #eee; }
.stage-bar .dot { width: 40rpx; height: 40rpx; border-radius: 50%; background: #e5e7eb; color: #9ca3af; font-size: 22rpx; display: flex; align-items: center; justify-content: center; }
.stage-bar .dot.done { background: #4C6EF5; color: #fff; }
.stage-bar .dot.cur { background: #4C6EF5; color: #fff; box-shadow: 0 0 0 6rpx rgba(76,110,245,.2); }
.stage-label { font-size: 24rpx; color: #6b7280; margin-left: auto; }
.msgs { flex: 1; padding: 24rpx; box-sizing: border-box; }
.msg { display: flex; margin-bottom: 24rpx; flex-direction: column; align-items: flex-start; }
.msg.user { align-items: flex-end; }
.bubble { max-width: 78%; padding: 20rpx 26rpx; border-radius: 18rpx; font-size: 28rpx; line-height: 1.6; white-space: pre-wrap; word-break: break-all; }
.msg.assistant .bubble { background: #fff; color: #1f2937; border-top-left-radius: 4rpx; }
.msg.user .bubble { background: #4C6EF5; color: #fff; border-top-right-radius: 4rpx; }
.chips { display: flex; flex-wrap: wrap; gap: 12rpx; margin-top: 12rpx; justify-content: flex-start; }
.chip { background: #eef2ff; color: #4C6EF5; border: none; font-size: 24rpx; border-radius: 999rpx; padding: 0 24rpx; }
.select-card { background: #fff; border-radius: 20rpx; padding: 28rpx; margin: 12rpx 0 24rpx; }
.card-title { font-size: 28rpx; color: #1f2937; font-weight: 600; display: block; margin-bottom: 20rpx; }
.opts { display: flex; flex-wrap: wrap; gap: 14rpx; margin-bottom: 24rpx; }
.opt { background: #f3f4f6; color: #374151; border: 2rpx solid transparent; font-size: 24rpx; border-radius: 12rpx; }
.opt.on { background: #eef2ff; color: #4C6EF5; border-color: #4C6EF5; }
.primary { background: #4C6EF5; color: #fff; font-size: 28rpx; border-radius: 12rpx; }
.primary[disabled] { background: #c7d2fe; }
.ghost { background: #fff; color: #6b7280; border: 1rpx solid #d1d5db; font-size: 26rpx; border-radius: 12rpx; margin-top: 12rpx; }
.input-bar { display: flex; gap: 16rpx; padding: 16rpx 24rpx calc(16rpx + env(safe-area-inset-bottom)); background: #fff; border-top: 1rpx solid #eee; }
.input { flex: 1; background: #f3f4f6; border-radius: 999rpx; padding: 14rpx 28rpx; font-size: 28rpx; }
.send { background: #4C6EF5; color: #fff; font-size: 26rpx; border-radius: 999rpx; padding: 0 36rpx; }
.send[disabled] { background: #c7d2fe; }
</style>
