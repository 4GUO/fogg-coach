/**
 * 统一请求层：token 管理、401 自动重登、多端复用（§6.0）
 */
const BASE = 'http://127.0.0.1:8080/api' // 开发期本地；生产走同域 /api

export function getToken(): string {
	return uni.getStorageSync('fc_token') || ''
}
export function setToken(t: string) {
	uni.setStorageSync('fc_token', t)
}

export interface ApiError extends Record<string, any> {
	code: number
	message?: string
}

function rawRequest(opts: any): Promise<any> {
	return new Promise((resolve, reject) => {
		uni.request({ ...opts, success: resolve, fail: reject })
	})
}

export async function api(path: string, body?: any, method?: string): Promise<any> {
	const hasBody = body !== undefined
	const header: Record<string, string> = { 'Content-Type': 'application/json' }
	const t = getToken()
	if (t) header.Authorization = 'Bearer ' + t

	let r: any
	try {
		r = await rawRequest({ url: BASE + path, method: method || (hasBody ? 'POST' : 'GET'), data: body, header })
	} catch {
		throw { code: -1, message: '网络不可用' } as ApiError
	}
	if (r.statusCode >= 200 && r.statusCode < 300) return r.data
	if (r.statusCode === 401 && !path.startsWith('/auth')) {
		const ok = await silentLogin() // 小程序静默重登（§3.6.2）
		if (ok) return api(path, body, method)
	}
	throw { code: r.statusCode, ...(r.data || {}), message: (r.data && r.data.message) || `HTTP ${r.statusCode}` } as ApiError
}

/** 登录：小程序 wx.login；H5/开发环境用 mock code（后端 WX_MOCK=1） */
export async function silentLogin(): Promise<boolean> {
	let code = ''
	// #ifdef MP-WEIXIN
	code = await new Promise<string>((resolve) => {
		uni.login({ provider: 'weixin', success: (r: any) => resolve(r.code), fail: () => resolve('') })
	})
	// #endif
	// #ifndef MP-WEIXIN
	code = 'h5-dev-' + Date.now()
	// #endif
	if (!code) return false
	try {
		const r = await rawRequest({
			url: BASE + '/auth/login', method: 'POST',
			data: { provider: 'wechat_mp', code },
			header: { 'Content-Type': 'application/json' },
		})
		if (r.statusCode === 200 && r.data && r.data.token) {
			setToken(r.data.token)
			uni.setStorageSync('fc_user', r.data.user)
			return true
		}
		return false
	} catch {
		return false
	}
}

export async function ensureLogin(): Promise<boolean> {
	return !!getToken() || (await silentLogin())
}

export { BASE }
