<template>
	<view class="chat-page">
		<!-- 弱网提示条（T2.4a）：不打断、不弹窗 -->
		<view v-if="connState !== 'ok'" class="conn-bar" :class="connState" @click="connState === 'failed' && retryLast()">
			<text>{{ connText }}</text>
		</view>

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
				<text class="card-title">计划准备好了，这是你的实验，不是承诺</text>
				<button class="primary" @click="doConfirm">确认，开始我的计划</button>
				<button class="ghost" @click="doRevise">再改改</button>
			</view>
		</scroll-view>

		<!-- 输入区 -->
		<view class="input-bar">
			<input v-model="draft" class="input" :disabled="busy" confirm-type="send" maxlength="500"
				placeholder="说说你的想法…" @confirm="send(draft)" />
			<button class="send" :disabled="busy || !draft.trim()" @click="send(draft)">发送</button>
		</view>
	</view>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api, ensureLogin } from '../../utils/request'
import { sendChat, isRecoverable } from '../../composables/useChatStream'
import type { ChatEvent, ChatPayload } from '../../composables/useChatStream'

const stages = ['S1', 'S2', 'S3', 'S4', 'S5', 'S6', 'S7']
const stageNames = ['探索愿望', '了解现状', '头脑风暴', '挑选行为', '设计配方', '确认计划', '生成计划']

/** T2.4a 断流重连参数：最多 3 次，指数退避 1s/2s/4s */
const MAX_RETRIES = 3
const RETRY_DELAYS = [1000, 2000, 4000]
/** 断流后等待后端落库的额外观察窗（content 已入库但回复未落库时，先等再判失败） */
const PENDING_RECHECK_MS = 2000

interface Msg { role: string; content: string; chips?: string[] }
const messages = ref<Msg[]>([])
const draft = ref('')
const stage = ref('S1')
const sessionId = ref<string | null>(null)
const candidates = ref<string[]>([])
const picked = ref<number[]>([])
const busy = ref(false)
const scrollTop = ref(0)

/** 弱网提示条状态：ok | connecting(连接中) | reconnected(已重连，短暂展示) | failed(点击重试) */
const connState = ref<'ok' | 'connecting' | 'reconnected' | 'failed'>('ok')
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
const connText = computed(() => ({
	connecting: '连接中…',
	reconnected: '已重连',
	failed: '连接中断，点击重试',
}[connState.value] || ''))

/** 最近一次失败的轮次（点击重试用） */
let lastFailed: { payload: ChatPayload; beforeTail: string } | null = null

onMounted(restore)
onUnmounted(() => { if (reconnectTimer) clearTimeout(reconnectTimer) })

function flashReconnected() {
	connState.value = 'reconnected'
	if (reconnectTimer) clearTimeout(reconnectTimer)
	reconnectTimer = setTimeout(() => { connState.value = 'ok' }, 2000)
}

/** /session/active → 本地渲染（初次恢复与断流补渲染共用） */
function applyActive(r: any) {
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
	if (hist.length) messages.value = hist
	scrollBottom()
}

async function restore() {
	if (!(await ensureLogin())) { messages.value = [{ role: 'assistant', content: '登录失败，请稍后再试' }]; return }
	try {
		applyActive(await api('/session/active'))
	} catch {
		// 404 = 无进行中会话，开新对话
	}
	if (!messages.value.length) {
		messages.value = [{ role: 'assistant', content: '你好呀，我是小福，你的行为设计教练 🌱\n最近你希望生活里有什么不一样？' }]
	}
	scrollBottom()
}

/** 本地消息尾部最后一条 assistant 内容（用于检测"该轮是否已被后端完整处理"） */
function tailAssistant(): string {
	for (let i = messages.value.length - 1; i >= 0; i--) {
		if (messages.value[i].role === 'assistant') return messages.value[i].content
	}
	return ''
}

/**
 * 断流后拉会话现状，判断下一步：
 *  - done    该轮已被后端完整处理 → 调用方直接补渲染
 *  - resend  本轮消息没进历史 → 安全重发原 payload
 *  - wait    content 已入库但回复未落库（后端可能还在写）→ 稍后再查
 */
async function assess(r: any, payload: ChatPayload, beforeTail: string): Promise<'done' | 'resend' | 'wait'> {
	const msgs: any[] = r.messages || []
	if (payload.content) {
		const content = payload.content.trim()
		const norm = [...content].slice(0, 500).join('') // 服务端可能已 500 rune 截断落库
		let idx = -1
		for (let i = msgs.length - 1; i >= 0; i--) {
			const c = msgs[i].content.trim()
			if (msgs[i].role === 'user' && (c === content || c === norm)) { idx = i; break }
		}
		if (idx < 0) return 'resend' // 没进历史 → 重发
		return msgs.slice(idx + 1).some((m) => m.role === 'assistant') ? 'done' : 'wait'
	}
	// 纯 action 轮（select/confirm/revise）：看 stage 是否推进 / 是否出现新 assistant 回复
	if (r.session && r.session.stage && r.session.stage !== stage.value) return 'done'
	const tail = (() => { for (let i = msgs.length - 1; i >= 0; i--) if (msgs[i].role === 'assistant') return msgs[i].content; return '' })()
	if (tail && tail !== beforeTail) return 'done'
	return 'resend' // action 未生效，重发 action（不产生重复 user 消息）
}

const sleep = (ms: number) => new Promise<void>((res) => setTimeout(res, ms))

/** 单次 SSE + 事件应用 */
async function streamOnce(payload: ChatPayload): Promise<ChatEvent[]> {
	const ai = { role: 'assistant', content: '' }
	messages.value.push(ai)
	scrollBottom()
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
	return evs
}

/** 带断流重连的流式入口（T2.4a） */
async function stream(payload: ChatPayload) {
	busy.value = true
	lastFailed = null
	const beforeTail = tailAssistant()
	let current = payload
	try {
		for (let attempt = 0; ; attempt++) {
			try {
				connState.value = 'ok'
				await streamOnce(current)
				return
			} catch (err: any) {
				// 清掉本轮残留的空 assistant 气泡，避免重试时叠出多条
				const tail = messages.value[messages.value.length - 1]
				if (tail && tail.role === 'assistant' && !tail.content) messages.value.pop()
				if (!isRecoverable(err) || attempt >= MAX_RETRIES) {
					// 不可恢复错误 or 3 次重连均失败：明确失败态
					connState.value = 'failed'
					messages.value.push({ role: 'assistant', content: '⚠️ ' + (err?.message || '网络开小差了') })
					lastFailed = { payload: current, beforeTail }
					scrollBottom()
					return
				}
				// 自动重连：提示条 → 退避 → 拉会话现状 → 补渲染 / 重发
				connState.value = 'connecting'
				await sleep(RETRY_DELAYS[Math.min(attempt, RETRY_DELAYS.length - 1)])
				try {
					const r = await api('/session/active')
					const verdict = await assess(r, current, beforeTail)
					if (verdict === 'done') {
						applyActive(r) // 后端已完整处理，直接补渲染
						flashReconnected()
						return
					}
					if (verdict === 'wait') {
						await sleep(PENDING_RECHECK_MS)
						const r2 = await api('/session/active')
						if ((await assess(r2, current, beforeTail)) === 'done') {
							applyActive(r2)
							flashReconnected()
							return
						}
						if (attempt + 1 >= MAX_RETRIES) { // 仍无回复 → 报失败，绝不重发（防重复 user 消息）
							messages.value.push({ role: 'assistant', content: '⚠️ 教练回复丢失了，请点击上方重试' })
							connState.value = 'failed'
							lastFailed = { payload: current, beforeTail }
							scrollBottom()
							return
						}
						continue
					}
					// resend：消息确实没进历史，重发原 payload（流式重跑）
					current = payload
				} catch {
					// /session/active 也失败：算一次重连失败，继续退避
				}
			}
		}
	} finally {
		busy.value = false
		scrollBottom()
	}
}

/** 失败态"点击重试"：重新走一轮恢复判断 */
function retryLast() {
	if (!lastFailed || busy.value) return
	const { payload, beforeTail } = lastFailed
	busy.value = true
	connState.value = 'connecting'
	;(async () => {
		try {
			const r = await api('/session/active')
			const verdict = await assess(r, payload, beforeTail)
			if (verdict === 'done') {
				applyActive(r)
				flashReconnected()
				return
			}
			if (verdict === 'wait') {
				await sleep(PENDING_RECHECK_MS)
				const r2 = await api('/session/active')
				if ((await assess(r2, payload, beforeTail)) === 'done') {
					applyActive(r2)
					flashReconnected()
					return
				}
				connState.value = 'failed'
				return
			}
			// 没进历史 → 安全重发
			await stream(payload)
		} catch {
			connState.value = 'failed'
		} finally {
			busy.value = false
			scrollBottom()
		}
	})()
}

async function send(text: string) {
	const raw = (text || '').trim()
	// 与服务端时齐：500 rune 截断（防断流重连后判重失效致重复入库）
	const content = [...raw].slice(0, 500).join('')
	if (!content || busy.value) return
	draft.value = ''
	messages.value.push({ role: 'user', content })
	await stream({ sessionId: sessionId.value, content })
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
		messages.value.push({ role: 'assistant', content: '你的计划生成好了 🌱\n到「今日」页看看吧，从今晚就可以开始第一个小实验。' })
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
.chat-page { display: flex; flex-direction: column; height: 100vh; background: var(--fc-bg, #F7F7F8); }

/* 弱网提示条：细、克制、不打断 */
.conn-bar { padding: 10rpx 24rpx; font-size: 24rpx; text-align: center; }
.conn-bar.connecting { background: var(--fc-primary-weak, #EEF2FF); color: var(--fc-primary, #4C6EF5); }
.conn-bar.reconnected { background: #E6FCF5; color: var(--fc-green, #2F9E6E); }
.conn-bar.failed { background: #F2F3F5; color: var(--fc-text-sub, #86909C); }

.stage-bar { display: flex; align-items: center; gap: 10rpx; padding: 16rpx 24rpx; background: var(--fc-card, #fff); border-bottom: 1rpx solid var(--fc-border, #E5E6EB); }
.stage-bar .dot { width: 40rpx; height: 40rpx; border-radius: 50%; background: #F2F3F5; color: var(--fc-text-sub, #86909C); font-size: 22rpx; display: flex; align-items: center; justify-content: center; }
.stage-bar .dot.done { background: var(--fc-primary, #4C6EF5); color: #fff; }
.stage-bar .dot.cur { background: var(--fc-primary, #4C6EF5); color: #fff; box-shadow: 0 0 0 6rpx rgba(76, 110, 245, .18); }
.stage-label { font-size: 24rpx; color: var(--fc-text-sub, #86909C); margin-left: auto; }
.msgs { flex: 1; padding: 24rpx; box-sizing: border-box; }
.msg { display: flex; margin-bottom: 24rpx; flex-direction: column; align-items: flex-start; }
.msg.user { align-items: flex-end; }
.bubble { max-width: 78%; padding: 20rpx 26rpx; border-radius: 20rpx; font-size: 28rpx; line-height: 1.6; white-space: pre-wrap; word-break: break-all; }
.msg.assistant .bubble { background: var(--fc-card, #fff); color: var(--fc-text, #1F2329); border-top-left-radius: 6rpx; border: 1rpx solid var(--fc-border, #E5E6EB); }
.msg.user .bubble { background: var(--fc-primary, #4C6EF5); color: #fff; border-top-right-radius: 6rpx; }
.chips { display: flex; flex-wrap: wrap; gap: 12rpx; margin-top: 12rpx; justify-content: flex-start; }
.chip { background: var(--fc-primary-weak, #EEF2FF); color: var(--fc-primary, #4C6EF5); border: none; font-size: 24rpx; border-radius: 999rpx; padding: 0 24rpx; }
.select-card { background: var(--fc-card, #fff); border-radius: 20rpx; padding: 28rpx; margin: 12rpx 0 24rpx; border: 1rpx solid var(--fc-border, #E5E6EB); }
.card-title { font-size: 28rpx; color: var(--fc-text, #1F2329); font-weight: 600; display: block; margin-bottom: 20rpx; }
.opts { display: flex; flex-wrap: wrap; gap: 14rpx; margin-bottom: 24rpx; }
.opt { background: var(--fc-bg, #F7F7F8); color: var(--fc-text, #1F2329); border: 2rpx solid transparent; font-size: 24rpx; border-radius: 16rpx; }
.opt.on { background: var(--fc-primary-weak, #EEF2FF); color: var(--fc-primary, #4C6EF5); border-color: var(--fc-primary, #4C6EF5); }
.primary { background: var(--fc-primary, #4C6EF5); color: #fff; font-size: 28rpx; border-radius: 16rpx; }
.primary[disabled] { background: var(--fc-primary-disabled, #C7D2FE); }
.ghost { background: var(--fc-card, #fff); color: var(--fc-text-sub, #86909C); border: 1rpx solid var(--fc-border, #E5E6EB); font-size: 26rpx; border-radius: 16rpx; margin-top: 12rpx; }
.input-bar { display: flex; gap: 16rpx; padding: 16rpx 24rpx calc(16rpx + env(safe-area-inset-bottom)); background: var(--fc-card, #fff); border-top: 1rpx solid var(--fc-border, #E5E6EB); }
.input { flex: 1; background: var(--fc-bg, #F7F7F8); border-radius: 999rpx; padding: 14rpx 28rpx; font-size: 28rpx; }
.send { background: var(--fc-primary, #4C6EF5); color: #fff; font-size: 26rpx; border-radius: 999rpx; padding: 0 36rpx; }
.send[disabled] { background: var(--fc-primary-disabled, #C7D2FE); }
</style>
