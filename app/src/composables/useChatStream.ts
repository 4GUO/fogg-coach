/**
 * SSE 流式对话封装（system-design §6.0）：
 *  - 微信小程序：uni.request enableChunked + onChunkReceived
 *  - H5/安卓：fetch ReadableStream（EventSource 不能带 header）
 *
 * T2.4a 弱网加固：
 *  - 空闲超时看门狗：IDLE_TIMEOUT_MS 内无任何 delta/事件 → 主动 abort，抛 IDLE_TIMEOUT
 *  - 双端手动 abort：小程序 task.abort()；H5 AbortController
 *  - 重连策略（拉会话现状 → 补渲染/重发）由调用方 chat.vue 实现（需要 /session/active 上下文）
 */
import { BASE, getToken } from '../utils/request'

/** 空闲超时：N 秒无任何 delta/事件视为断流（可配置） */
export const IDLE_TIMEOUT_MS = 15_000
const WATCHDOG_TICK = 1_000

export interface ChatEvent {
	delta?: string
	event?: string // stage_done | quick_replies
	next?: string
	items?: string[]
	done?: boolean
	sessionId?: string
	stage?: string
	messageId?: string
	candidates?: string[]
	forcePlan?: boolean
	error?: string
	message?: string
}

export interface ChatHandlers {
	onDelta?: (t: string) => void
	onEvent?: (e: ChatEvent) => void
	/** 连接状态回调（弱网提示条用） */
	onStatus?: (s: 'streaming' | 'aborted') => void
}

export interface ChatPayload {
	sessionId?: string | null
	content?: string
	action?: { type: string; options?: string[] } | null
}

export interface ChatStreamError extends Record<string, any> {
	code: string | number
	message?: string
}

/** 判断错误是否值得触发"断流重连"（网络层/超时类）；HTTP 4xx 业务错不重连 */
export function isRecoverable(err: any): boolean {
	if (!err) return false
	if (err.code === 'IDLE_TIMEOUT') return true
	if (err.code === -1 || err.code === 'NETWORK') return true
	// 小程序 fail 回调 / fetch 网络异常
	if (err && typeof err.code === 'undefined' && (err.errMsg || err instanceof TypeError)) return true
	if (typeof err.code === 'number' && err.code >= 500) return true
	return false
}

export interface ChatStreamOptions {
	idleTimeoutMs?: number
}

export async function sendChat(payload: ChatPayload, h: ChatHandlers, opts?: ChatStreamOptions): Promise<ChatEvent[]> {
	const events: ChatEvent[] = []
	const idleMs = opts?.idleTimeoutMs ?? IDLE_TIMEOUT_MS
	let lastActivity = Date.now()
	let gotDone = false
	let buffer = ''
	let watchdog: ReturnType<typeof setInterval> | null = null
	let aborter: (() => void) | null = null

	const stopWatchdog = () => { if (watchdog) { clearInterval(watchdog); watchdog = null } }

	const feed = (chunk: string) => {
		lastActivity = Date.now()
		// SSE 帧可能被 TCP 切开，按行缓冲
		buffer += chunk
		const lines = buffer.split('\n')
		buffer = lines.pop() || ''
		for (const line of lines) {
			const s = line.trim()
			if (!s.startsWith('data:')) continue
			try {
				const e = JSON.parse(s.slice(5).trim())
				events.push(e)
				if (e.done) gotDone = true
				if (e.delta) h.onDelta?.(e.delta)
				h.onEvent?.(e)
			} catch { /* 半包容错 */ }
		}
	}

	// 空闲看门狗：流式过程中 idleMs 无任何事件 → abort + 抛错（上游走重连）
	const startWatchdog = () => {
		watchdog = setInterval(() => {
			if (gotDone) { stopWatchdog(); return }
			if (Date.now() - lastActivity > idleMs) {
				stopWatchdog()
				h.onStatus?.('aborted')
				aborter?.()
			}
		}, WATCHDOG_TICK)
	}
	startWatchdog()

	try {
		// #ifdef MP-WEIXIN
		await new Promise<void>((resolve, reject) => {
			let settled = false
			const settle = (fn: () => void) => { if (!settled) { settled = true; fn() } }
			const task = uni.request({
				url: BASE + '/chat',
				method: 'POST',
				enableChunked: true,
				data: payload,
				header: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + getToken() },
				success: () => settle(resolve),
				fail: (e: any) => settle(() => reject({ code: 'NETWORK', message: (e && e.errMsg) || '网络不可用', ...e } as ChatStreamError)),
			} as any)
			aborter = () => {
				try { (task as any)?.abort?.() } catch { /* 已结束 */ }
				// abort 后 fail 回调不一定触发（基础库差异），这里兜底 reject
				settle(() => reject({ code: 'IDLE_TIMEOUT', message: '连接空闲超时' } as ChatStreamError))
			}
			// @ts-ignore 小程序专属 API
			task.onChunkReceived?.((res: any) => {
				feed(ab2str(res.data))
			})
		})
		// #endif

		// #ifndef MP-WEIXIN
		const ac = new AbortController()
		aborter = () => {
			try { ac.abort() } catch { /* 已结束 */ }
			// fetch abort 会让 reader.read() reject；此处不额外抛，交给 read 循环
		}
		const resp = await fetch(BASE + '/chat', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + getToken() },
			body: JSON.stringify(payload),
			signal: ac.signal,
		})
		if (!resp.ok) {
			let msg = `HTTP ${resp.status}`
			try { msg = (await resp.json()).message || msg } catch { /* ignore */ }
			throw { code: resp.status, message: msg } as ChatStreamError
		}
		const reader = (resp.body as ReadableStream<Uint8Array>).getReader()
		const dec = new TextDecoder()
		for (;;) {
			const { done, value } = await reader.read()
			if (done) break
			feed(dec.decode(value, { stream: true }))
		}
		// #endif
	} catch (err: any) {
		// H5 端 abort 会以 AbortError 冒泡，归一化为 IDLE_TIMEOUT
		if (aborter && err && err.name === 'AbortError') {
			throw { code: 'IDLE_TIMEOUT', message: '连接空闲超时' } as ChatStreamError
		}
		throw err
	} finally {
		stopWatchdog()
	}

	return events
}

/** ArrayBuffer → UTF-8 字符串（小程序端无 TextDecoder 时手写解码） */
function ab2str(ab: ArrayBuffer): string {
	// @ts-ignore 基础库 2.19+ 支持 TextDecoder，优先用
	if (typeof TextDecoder !== 'undefined') return new TextDecoder().decode(ab)
	const u8 = new Uint8Array(ab)
	let out = '', i = 0
	while (i < u8.length) {
		const b = u8[i]
		if (b < 0x80) { out += String.fromCharCode(b); i++ }
		else if (b < 0xe0) { out += String.fromCharCode(((b & 0x1f) << 6) | (u8[i + 1] & 0x3f)); i += 2 }
		else if (b < 0xf0) { out += String.fromCharCode(((b & 0x0f) << 12) | ((u8[i + 1] & 0x3f) << 6) | (u8[i + 2] & 0x3f)); i += 3 }
		else {
			const cp = ((b & 0x07) << 18) | ((u8[i + 1] & 0x3f) << 12) | ((u8[i + 2] & 0x3f) << 6) | (u8[i + 3] & 0x3f)
			const off = cp - 0x10000
			out += String.fromCharCode(0xd800 + (off >> 10), 0xdc00 + (off & 0x3ff))
			i += 4
		}
	}
	return out
}
