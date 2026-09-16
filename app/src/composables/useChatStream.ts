/**
 * SSE 流式对话封装（system-design §6.0）：
 *  - 微信小程序：uni.request enableChunked + onChunkReceived
 *  - H5/安卓：fetch ReadableStream（EventSource 不能带 header）
 */
import { BASE, getToken } from '../utils/request'

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
}

export interface ChatPayload {
	sessionId?: string | null
	content?: string
	action?: { type: string; options?: string[] } | null
}

export async function sendChat(payload: ChatPayload, h: ChatHandlers): Promise<ChatEvent[]> {
	const events: ChatEvent[] = []
	const feed = (chunk: string) => {
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
				if (e.delta) h.onDelta?.(e.delta)
				h.onEvent?.(e)
			} catch { /* 半包容错 */ }
		}
	}
	let buffer = ''

	// #ifdef MP-WEIXIN
	await new Promise<void>((resolve, reject) => {
		const task = uni.request({
			url: BASE + '/chat',
			method: 'POST',
			enableChunked: true,
			data: payload,
			header: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + getToken() },
			success: () => resolve(),
			fail: (e: any) => reject(e),
		} as any)
		// @ts-ignore 小程序专属 API
		task.onChunkReceived?.((res: any) => {
			feed(ab2str(res.data))
		})
	})
	// #endif

	// #ifndef MP-WEIXIN
	const resp = await fetch(BASE + '/chat', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + getToken() },
		body: JSON.stringify(payload),
	})
	if (!resp.ok) {
		let msg = `HTTP ${resp.status}`
		try { msg = (await resp.json()).message || msg } catch { /* ignore */ }
		throw { code: resp.status, message: msg }
	}
	const reader = (resp.body as ReadableStream<Uint8Array>).getReader()
	const dec = new TextDecoder()
	for (;;) {
		const { done, value } = await reader.read()
		if (done) break
		feed(dec.decode(value, { stream: true }))
	}
	// #endif

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
