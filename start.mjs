#!/usr/bin/env node
/**
 * DS2API 启动脚本 - 交互式菜单
 *
 * 使用方法:
 *   node start.mjs          # 显示交互式菜单
 *   node start.mjs dev      # 开发模式（后端 + 前端热重载）
 *   node start.mjs prod     # 生产模式（编译后运行）
 *   node start.mjs build    # 编译后端二进制
 *   node start.mjs webui    # 构建前端静态文件
 *   node start.mjs install  # 安装前端依赖
 *   node start.mjs stop     # 停止所有服务
 *   node start.mjs status   # 查看服务状态
 */

import { spawn, execSync } from 'child_process';
import { createInterface } from 'readline';
import { existsSync } from 'fs';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// 判断是否为 Windows
const isWindows = process.platform === 'win32';

// 编译产物路径
const BINARY = join(__dirname, isWindows ? 'ds2api.exe' : 'ds2api');

// 配置（从环境变量读取，与 Go 主程序保持一致）
const CONFIG = {
  port: process.env.PORT || '5001',
  frontendPort: 5173,
  logLevel: process.env.LOG_LEVEL || 'INFO',
  adminKey: process.env.DS2API_ADMIN_KEY || 'admin',
  webuiDir: join(__dirname, 'webui'),
  staticAdminDir: process.env.DS2API_STATIC_ADMIN_DIR || join(__dirname, 'static', 'admin'),
};

// 国内镜像配置
const MIRRORS = {
  goproxy: process.env.GOPROXY || 'https://goproxy.cn,direct',
  npm: process.env.NPM_REGISTRY || 'https://registry.npmmirror.com',
};

// 存储子进程
const processes = [];

// 颜色输出
const colors = {
  reset: '\x1b[0m',
  bright: '\x1b[1m',
  dim: '\x1b[2m',
  red: '\x1b[31m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  magenta: '\x1b[35m',
  cyan: '\x1b[36m',
};

const log = {
  info: (msg) => console.log(`${colors.cyan}[INFO]${colors.reset} ${msg}`),
  success: (msg) => console.log(`${colors.green}[OK]${colors.reset} ${msg}`),
  warn: (msg) => console.log(`${colors.yellow}[WARN]${colors.reset} ${msg}`),
  error: (msg) => console.log(`${colors.red}[ERROR]${colors.reset} ${msg}`),
  title: (msg) => console.log(`\n${colors.bright}${colors.magenta}${msg}${colors.reset}`),
};

// 清理并退出
function cleanup() {
  console.log('\n');
  log.info('正在关闭所有服务...');
  processes.forEach(proc => {
    if (proc && !proc.killed) {
      proc.kill('SIGTERM');
    }
  });
  log.success('已退出');
  process.exit(0);
}

process.on('SIGINT', cleanup);
process.on('SIGTERM', cleanup);

// 检查命令是否存在
function commandExists(cmd) {
  try {
    execSync(`${isWindows ? 'where' : 'which'} ${cmd}`, { stdio: 'ignore' });
    return true;
  } catch {
    return false;
  }
}

// 检查 Go 是否安装
function checkGo() {
  return commandExists('go');
}

// 获取 Go 版本
function getGoVersion() {
  try {
    return execSync('go version', { encoding: 'utf-8' }).trim();
  } catch {
    return null;
  }
}

// 检查前端依赖是否已安装
function checkFrontendDeps() {
  if (!existsSync(CONFIG.webuiDir)) return null;
  return existsSync(join(CONFIG.webuiDir, 'node_modules'));
}

// 检查前端是否已构建
function checkWebuiBuilt() {
  return existsSync(join(CONFIG.staticAdminDir, 'index.html'));
}

// 检查后端二进制是否存在
function binaryExists() {
  return existsSync(BINARY);
}

// 查找占用端口的进程 PID
function findPidByPort(port) {
  const numericPort = parseInt(port, 10);
  if (isNaN(numericPort)) return [];
  
  try {
    if (isWindows) {
      const output = execSync(`netstat -ano | findstr :${numericPort} | findstr LISTENING`, {
        encoding: 'utf-8',
        shell: true,
        stdio: ['pipe', 'pipe', 'ignore'],
      });
      const pids = new Set();
      for (const line of output.trim().split('\n')) {
        const parts = line.trim().split(/\s+/);
        const pid = parts[parts.length - 1];
        if (pid && pid !== '0') pids.add(pid);
      }
      return [...pids];
    } else {
      const output = execSync(`lsof -ti :${numericPort}`, {
        encoding: 'utf-8',
        stdio: ['pipe', 'pipe', 'ignore'],
      });
      return output.trim().split('\n').filter(Boolean);
    }
  } catch {
    return [];
  }
}

// 获取运行中的服务状态
function getRunningStatus() {
  const backendPids = findPidByPort(CONFIG.port);
  const frontendPids = findPidByPort(CONFIG.frontendPort);
  return {
    backend: backendPids,
    frontend: frontendPids,
    isRunning: backendPids.length > 0 || frontendPids.length > 0,
  };
}

// 停止服务
async function stopServices() {
  const running = getRunningStatus();

  if (!running.isRunning) {
    log.warn('没有检测到正在运行的服务');
    return;
  }

  log.title('========== 停止服务 ==========');

  const killProcess = async (pid) => {
    try {
      if (isWindows) {
        try {
          execSync(`taskkill /PID ${pid}`, { stdio: 'ignore', shell: true });
        } catch {
          execSync(`taskkill /F /T /PID ${pid}`, { stdio: 'ignore', shell: true });
        }
      } else {
        execSync(`kill -15 ${pid}`, { stdio: 'ignore' });
        await new Promise(r => setTimeout(r, 500));
        try {
          execSync(`kill -0 ${pid}`, { stdio: 'ignore' });
          execSync(`kill -9 ${pid}`, { stdio: 'ignore' });
        } catch { /* 进程已退出 */ }
      }
    } catch { /* 进程可能已退出 */ }
  };

  if (running.backend.length > 0) {
    log.info(`停止后端服务 (端口 ${CONFIG.port}, PID: ${running.backend.join(', ')})...`);
    for (const pid of running.backend) await killProcess(pid);
    log.success('后端服务已停止');
  }

  if (running.frontend.length > 0) {
    log.info(`停止前端服务 (端口 ${CONFIG.frontendPort}, PID: ${running.frontend.join(', ')})...`);
    for (const pid of running.frontend) await killProcess(pid);
    log.success('前端服务已停止');
  }
}

// 安装前端依赖
async function installFrontendDeps() {
  if (!existsSync(CONFIG.webuiDir)) {
    log.warn('webui 目录不存在，跳过前端依赖安装');
    return;
  }
  log.info(`安装前端依赖 (npm ci, registry: ${MIRRORS.npm})...`);
  return new Promise((resolve, reject) => {
    const proc = spawn('npm', ['ci', '--registry', MIRRORS.npm], {
      cwd: CONFIG.webuiDir,
      stdio: 'inherit',
      shell: isWindows,
    });
    proc.on('close', code => code === 0 ? resolve() : reject(new Error('前端依赖安装失败')));
  });
}

// 确保前端依赖已安装
async function ensureFrontendDeps() {
  if (checkFrontendDeps() === false) {
    log.warn('检测到前端依赖未安装，正在安装...');
    await installFrontendDeps();
  }
}

// 编译后端二进制
async function buildBackend() {
  if (!checkGo()) throw new Error('未找到 Go，请先安装 Go (https://go.dev/dl/)');
  log.info(`编译后端二进制 (GOPROXY: ${MIRRORS.goproxy})...`);
  return new Promise((resolve, reject) => {
    const proc = spawn('go', ['build', '-o', BINARY, './cmd/ds2api'], {
      cwd: __dirname,
      stdio: 'inherit',
      shell: isWindows,
      env: { ...process.env, GOPROXY: MIRRORS.goproxy },
    });
    proc.on('close', code => code === 0 ? resolve() : reject(new Error('后端编译失败')));
  });
}

// 构建前端静态文件
async function buildWebui() {
  if (!existsSync(CONFIG.webuiDir)) {
    log.warn('webui 目录不存在');
    return;
  }
  await ensureFrontendDeps();
  log.info('构建前端静态文件...');
  return new Promise((resolve, reject) => {
    const proc = spawn(
      'npm', ['run', 'build', '--', '--outDir', CONFIG.staticAdminDir, '--emptyOutDir'],
      { cwd: CONFIG.webuiDir, stdio: 'inherit', shell: isWindows }
    );
    proc.on('close', code => code === 0 ? resolve() : reject(new Error('前端构建失败')));
  });
}

// 启动后端（开发模式：go run，无需预编译）
async function startBackendDev() {
  if (!checkGo()) throw new Error('未找到 Go，请先安装 Go (https://go.dev/dl/)');
  log.info(`启动后端（go run）... 本地 http://127.0.0.1:${CONFIG.port}  绑定 0.0.0.0:${CONFIG.port}`);
  const proc = spawn('go', ['run', './cmd/ds2api'], {
    cwd: __dirname,
    stdio: 'inherit',
    shell: isWindows,
    env: { ...process.env,
      PORT: CONFIG.port,
      LOG_LEVEL: CONFIG.logLevel,
      DS2API_ADMIN_KEY: CONFIG.adminKey,
      GOPROXY: MIRRORS.goproxy,
    },
  });
  processes.push(proc);
  return proc;
}

// 启动后端（生产模式：运行编译好的二进制）
async function startBackendProd() {
  if (!binaryExists()) {
    log.warn('未找到编译产物，正在编译...');
    await buildBackend();
  }
  log.info(`启动后端（二进制）... 本地 http://127.0.0.1:${CONFIG.port}  绑定 0.0.0.0:${CONFIG.port}`);
  const proc = spawn(BINARY, [], {
    cwd: __dirname,
    stdio: 'inherit',
    shell: false,
    env: {
      ...process.env,
      PORT: CONFIG.port,
      LOG_LEVEL: CONFIG.logLevel,
      DS2API_ADMIN_KEY: CONFIG.adminKey,
    },
  });
  processes.push(proc);
  return proc;
}

// 启动前端开发服务器
async function startFrontend() {
  if (!existsSync(CONFIG.webuiDir)) {
    log.warn('webui 目录不存在，跳过前端启动');
    return null;
  }
  await ensureFrontendDeps();
  log.info(`启动前端开发服务器... http://localhost:${CONFIG.frontendPort}`);
  const proc = spawn('npm', ['run', 'dev'], {
    cwd: CONFIG.webuiDir,
    stdio: 'inherit',
    shell: true,
  });
  processes.push(proc);
  return proc;
}

// 显示状态信息
function showStatus() {
  console.log('\n' + '─'.repeat(50));
  log.success(`后端 API:  http://127.0.0.1:${CONFIG.port}`);
  log.success(`管理界面: http://127.0.0.1:${CONFIG.port}/admin`);
  log.info(`后端绑定:  0.0.0.0:${CONFIG.port} (可通过局域网 IP 访问)`);
  if (existsSync(CONFIG.webuiDir)) {
    log.success(`前端 Dev:  http://localhost:${CONFIG.frontendPort}`);
  }
  console.log('─'.repeat(50));
  log.info('按 Ctrl+C 停止所有服务\n');
}

// 等待进程退出
function waitForProcesses() {
  return new Promise(resolve => {
    const check = setInterval(() => {
      const activeCount = processes.filter(proc => proc.exitCode === null && proc.signalCode === null).length;
      if (activeCount === 0) {
        clearInterval(check);
        resolve();
      }
    }, 1000);
  });
}

// 交互式菜单
async function showMenu() {
  const rl = createInterface({ input: process.stdin, output: process.stdout });
  const question = (prompt) => new Promise(resolve => rl.question(prompt, resolve));

  console.clear();
  log.title('╔══════════════════════════════════════════╗');
  log.title('║         DS2API 启动脚本  (Go)            ║');
  log.title('╚══════════════════════════════════════════╝');

  // 环境状态
  const goVersion = getGoVersion();
  const frontendDeps = checkFrontendDeps();
  const webuiBuilt = checkWebuiBuilt();
  const hasBinary = binaryExists();
  const running = getRunningStatus();

  const ok = (v) => v ? `${colors.green}✓${colors.reset}` : `${colors.yellow}✗${colors.reset}`;

  console.log(`\n${colors.bright}环境状态:${colors.reset}`);
  console.log(`  Go:          ${goVersion ? `${colors.green}${goVersion}${colors.reset}` : `${colors.red}未安装${colors.reset}`}`);
  console.log(`  前端依赖:    ${frontendDeps === null ? `${colors.dim}N/A${colors.reset}` : frontendDeps ? `${colors.green}已安装${colors.reset}` : `${colors.yellow}未安装${colors.reset}`}`);
  console.log(`  前端构建:    ${ok(webuiBuilt)} ${webuiBuilt ? `(${CONFIG.staticAdminDir})` : '未构建'}`);
  console.log(`  后端二进制:  ${ok(hasBinary)} ${hasBinary ? BINARY : '未编译'}`);

  console.log(`\n${colors.bright}服务状态:${colors.reset}`);
  console.log(`  后端 (:${CONFIG.port}):    ${running.backend.length > 0 ? `${colors.green}运行中${colors.reset} (PID: ${running.backend.join(', ')})` : `${colors.dim}未运行${colors.reset}`}`);
  console.log(`  前端 (:${CONFIG.frontendPort}): ${running.frontend.length > 0 ? `${colors.green}运行中${colors.reset} (PID: ${running.frontend.join(', ')})` : `${colors.dim}未运行${colors.reset}`}`);

  console.log(`\n${colors.bright}环境变量:${colors.reset}`);
  console.log(`  PORT:              ${colors.cyan}${CONFIG.port}${colors.reset}`);
  console.log(`  LOG_LEVEL:         ${colors.cyan}${CONFIG.logLevel}${colors.reset}`);
  console.log(`  DS2API_ADMIN_KEY:  ${colors.cyan}${CONFIG.adminKey}${colors.reset}`);
  console.log(`  GOPROXY:           ${colors.cyan}${MIRRORS.goproxy}${colors.reset}`);
  console.log(`  NPM_REGISTRY:      ${colors.cyan}${MIRRORS.npm}${colors.reset}`);
  console.log(`${colors.dim}  自定义: DS2API_ADMIN_KEY=密钥 PORT=5001 node start.mjs${colors.reset}`);

  console.log(`
${colors.bright}请选择操作:${colors.reset}

  ${colors.cyan}1.${colors.reset} 开发模式  (go run + 前端热重载)
  ${colors.cyan}2.${colors.reset} 仅后端    (go run，无需编译)
  ${colors.cyan}3.${colors.reset} 仅前端    (npm dev)
  ${colors.cyan}4.${colors.reset} 生产模式  (编译后运行，前端已嵌入)
  ${colors.cyan}5.${colors.reset} 编译后端  (go build)
  ${colors.cyan}6.${colors.reset} 构建前端  (npm build → static/admin)
  ${colors.cyan}7.${colors.reset} 安装前端依赖 (npm ci)
  ${colors.red}8.${colors.reset} 停止所有服务
  ${colors.cyan}0.${colors.reset} 退出
`);

  const choice = await question(`${colors.yellow}请输入选项 [1]: ${colors.reset}`);
  rl.close();

  switch (choice.trim() || '1') {
    case '1':
      log.title('========== 开发模式 ==========');
      await startBackendDev();
      await new Promise(r => setTimeout(r, 1500));
      await startFrontend();
      showStatus();
      await waitForProcesses();
      break;

    case '2':
      log.title('========== 仅后端 (go run) ==========');
      await startBackendDev();
      showStatus();
      await waitForProcesses();
      break;

    case '3':
      log.title('========== 仅前端 ==========');
      await startFrontend();
      showStatus();
      await waitForProcesses();
      break;

    case '4':
      log.title('========== 生产模式 ==========');
      await startBackendProd();
      showStatus();
      await waitForProcesses();
      break;

    case '5':
      log.title('========== 编译后端 ==========');
      await buildBackend();
      log.success(`编译完成：${BINARY}`);
      break;

    case '6':
      log.title('========== 构建前端 ==========');
      await buildWebui();
      log.success('前端构建完成！');
      break;

    case '7':
      log.title('========== 安装前端依赖 ==========');
      await installFrontendDeps();
      log.success('前端依赖安装完成！');
      break;

    case '8':
      await stopServices();
      break;

    case '0':
      log.info('再见！');
      process.exit(0);
      break;

    default:
      log.warn('无效选项');
      await showMenu();
  }
}

// 命令行参数处理
async function main() {
  const cmd = process.argv[2];

  if (!checkGo() && !['install', 'webui', 'stop', 'status', 'help', '-h', '--help'].includes(cmd)) {
    log.error('未找到 Go，请先安装 Go: https://go.dev/dl/');
    if (!cmd) {
      // 无 Go 时仍允许进入菜单（可以只操作前端）
    } else {
      process.exit(1);
    }
  }

  switch (cmd) {
    case 'dev':
      log.title('========== 开发模式 ==========');
      await startBackendDev();
      await new Promise(r => setTimeout(r, 1500));
      await startFrontend();
      showStatus();
      await waitForProcesses();
      break;

    case 'prod':
      log.title('========== 生产模式 ==========');
      await startBackendProd();
      showStatus();
      await waitForProcesses();
      break;

    case 'build':
      await buildBackend();
      log.success(`编译完成：${BINARY}`);
      break;

    case 'webui':
      await buildWebui();
      log.success('前端构建完成！');
      break;

    case 'install':
      await installFrontendDeps();
      log.success('前端依赖安装完成！');
      break;

    case 'stop':
      await stopServices();
      break;

    case 'status': {
      const status = getRunningStatus();
      const goVer = getGoVersion();
      console.log(`\n${colors.bright}环境:${colors.reset}`);
      console.log(`  Go: ${goVer || `${colors.red}未安装${colors.reset}`}`);
      console.log(`\n${colors.bright}服务状态:${colors.reset}`);
      console.log(`  后端 (:${CONFIG.port}):    ${status.backend.length > 0 ? `${colors.green}运行中${colors.reset} (PID: ${status.backend.join(', ')})` : `${colors.dim}未运行${colors.reset}`}`);
      console.log(`  前端 (:${CONFIG.frontendPort}): ${status.frontend.length > 0 ? `${colors.green}运行中${colors.reset} (PID: ${status.frontend.join(', ')})` : `${colors.dim}未运行${colors.reset}`}\n`);
      break;
    }

    case 'help':
    case '-h':
    case '--help':
      console.log(`
${colors.bright}DS2API 启动脚本 (Go)${colors.reset}

${colors.cyan}使用方法:${colors.reset}
  node start.mjs              显示交互式菜单
  node start.mjs dev          开发模式 (go run + 前端热重载)
  node start.mjs prod         生产模式 (编译产物，前端已嵌入)
  node start.mjs build        编译后端二进制 (go build)
  node start.mjs webui        构建前端静态文件
  node start.mjs install      安装前端依赖 (npm ci)
  node start.mjs stop         停止所有服务
  node start.mjs status       查看服务状态

${colors.cyan}常用环境变量:${colors.reset}
  PORT               后端端口 (默认: 5001)
  LOG_LEVEL          日志级别: DEBUG|INFO|WARN|ERROR (默认: INFO)
  DS2API_ADMIN_KEY   管理员密钥 (默认: admin)
  DS2API_CONFIG_PATH 配置文件路径 (默认: config.json)
  GOPROXY            Go 模块代理 (默认: https://goproxy.cn,direct)
  NPM_REGISTRY       npm 镜像源 (默认: https://registry.npmmirror.com)

${colors.cyan}示例:${colors.reset}
  DS2API_ADMIN_KEY=mykey PORT=8080 node start.mjs dev
  GOPROXY=off NPM_REGISTRY=https://registry.npmjs.org node start.mjs dev
`);
      break;

    default:
      await showMenu();
  }
}

main().catch(e => {
  log.error(e.message);
  process.exit(1);
});
