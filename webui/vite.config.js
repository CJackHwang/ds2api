import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const webuiDir = dirname(fileURLToPath(import.meta.url))
const repoRoot = resolve(webuiDir, '..')

export default defineConfig(({ mode }) => {
    const env = loadEnv(mode, webuiDir, '')
    const backendUrl = env.VITE_API_BASE_URL || 'http://localhost:5001'

    return ({
    plugins: [
        react(),
    ],
    server: {
        port: 5173,
        fs: {
            allow: [repoRoot],
        },
        proxy: {
            // 代理 /admin 下的 API 请求到后端
            '/admin': {
                target: backendUrl,
                changeOrigin: true,
                // 只代理 API 请求，页面请求返回 false 让 Vite 处理
                bypass(req, res, proxyOptions) {
                    const url = req.url
                    // 精确的 /admin 或 /admin/ 是页面请求，不代理
                    if (url === '/admin' || url === '/admin/' || url === '/admin?') {
                        console.log('[Vite Proxy] Bypass (page):', url)
                        return '/index.html'
                    }
                    // 其他 /admin/* 路径都是 API 请求，代理到后端
                    console.log('[Vite Proxy] Proxy to backend:', url)
                    // 返回 undefined 或 null 表示不跳过代理
                },
            },
            '/v1': {
                target: backendUrl,
                changeOrigin: true,
            },
        },
    },
    build: {
        outDir: '../static/admin',
        emptyOutDir: true,
    },
    // Use / for dev, /admin/ for production build
    base: mode === 'production' ? '/admin/' : '/',
    })
})
