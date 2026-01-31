# -*- coding: utf-8 -*-
"""账号认证与管理模块 - 轮询(Round-Robin)策略"""
import threading
from fastapi import HTTPException, Request

from .config import CONFIG, logger
from .deepseek import login_deepseek_via_account, BASE_HEADERS

# -------------------------- 全局账号队列 --------------------------
# 使用列表实现轮询队列，配合线程锁保证并发安全
account_queue = []  # 可用账号队列
in_use_accounts = {}  # 正在使用的账号 {account_id: account}
_queue_lock = threading.Lock()  # 线程锁

claude_api_key_queue = []  # 维护所有可用的Claude API keys


def init_account_queue():
    """初始化时从配置加载账号（不再随机排序，保持配置顺序）"""
    global account_queue, in_use_accounts
    with _queue_lock:
        account_queue = CONFIG.get("accounts", [])[:]  # 深拷贝
        in_use_accounts = {}
        # 按 token 有无排序：有 token 的账号优先
        account_queue.sort(key=lambda a: 0 if a.get("token", "").strip() else 1)
        logger.info(f"[init_account_queue] 初始化 {len(account_queue)} 个账号，轮询模式")


def init_claude_api_key_queue():
    """Claude API keys由用户自己的token提供，这里初始化为空"""
    global claude_api_key_queue
    claude_api_key_queue = []


# 初始化
init_account_queue()
init_claude_api_key_queue()


# ----------------------------------------------------------------------
# 辅助函数：获取账号唯一标识（优先 email，否则 mobile）
# ----------------------------------------------------------------------
def get_account_identifier(account: dict) -> str:
    """返回账号的唯一标识，优先使用 email，否则使用 mobile"""
    return account.get("email", "").strip() or account.get("mobile", "").strip()


def get_queue_status() -> dict:
    """获取账号队列状态（用于监控）"""
    with _queue_lock:
        # total 应该是配置中的账号总数，而非队列相加（避免状态不一致导致重复计数）
        total_accounts = len(CONFIG.get("accounts", []))
        return {
            "available": len(account_queue),
            "in_use": len(in_use_accounts),
            "total": total_accounts,
            "available_accounts": [get_account_identifier(a) for a in account_queue],
            "in_use_accounts": list(in_use_accounts.keys()),
        }


# ----------------------------------------------------------------------
# 账号选择与释放 - 轮询(Round-Robin)策略
# ----------------------------------------------------------------------
def choose_new_account(exclude_ids=None):
    """轮询选择策略：
    1. 使用线程锁保证并发安全
    2. 优先选择队首的有 token 账号
    3. 从队列头部取出账号（FIFO）
    4. 请求完成后调用 release_account 将账号放回队尾
    """
    if exclude_ids is None:
        exclude_ids = []

    with _queue_lock:
        # 第一轮：优先选择已有 token 的账号
        for i in range(len(account_queue)):
            acc = account_queue[i]
            acc_id = get_account_identifier(acc)
            if acc_id and acc_id not in exclude_ids:
                if acc.get("token", "").strip():  # 已有 token
                    selected = account_queue.pop(i)
                    in_use_accounts[acc_id] = selected
                    logger.info(f"[choose_new_account] 轮询选择(有token): {acc_id} | 队列剩余: {len(account_queue)}")
                    return selected

        # 第二轮：选择任意账号（需要登录）
        for i in range(len(account_queue)):
            acc = account_queue[i]
            acc_id = get_account_identifier(acc)
            if acc_id and acc_id not in exclude_ids:
                selected = account_queue.pop(i)
                in_use_accounts[acc_id] = selected
                logger.info(f"[choose_new_account] 轮询选择(需登录): {acc_id} | 队列剩余: {len(account_queue)}")
                return selected

        logger.warning(f"[choose_new_account] 没有可用账号 | 队列: {len(account_queue)}, 使用中: {len(in_use_accounts)}")
        return None


def release_account(account: dict):
    """将账号重新加入队列末尾（轮询核心：用完放队尾）"""
    if not account:
        return
    
    acc_id = get_account_identifier(account)
    with _queue_lock:
        # 从使用中移除
        if acc_id in in_use_accounts:
            del in_use_accounts[acc_id]
        
        # 放回队尾
        account_queue.append(account)
        logger.debug(f"[release_account] 释放账号: {acc_id} | 队列长度: {len(account_queue)}")


# ----------------------------------------------------------------------
# Claude API key 管理函数（简化版本）
# ----------------------------------------------------------------------
def choose_claude_api_key():
    """选择一个可用的Claude API key - 现在直接由用户提供"""
    return None


def release_claude_api_key(api_key):
    """释放Claude API key - 现在无需操作"""
    pass


# ----------------------------------------------------------------------
# 判断调用模式：配置模式 vs 用户自带 token
# ----------------------------------------------------------------------
def determine_mode_and_token(request: Request):
    """
    根据请求头 Authorization 判断使用哪种模式：
    - 如果 Bearer token 出现在 CONFIG["keys"] 中，则为配置模式，从 CONFIG["accounts"] 中随机选择一个账号（排除已尝试账号），
      检查该账号是否已有 token，否则调用登录接口获取；
    - 否则，直接使用请求中的 Bearer 值作为 DeepSeek token。
    结果存入 request.state.deepseek_token；配置模式下同时存入 request.state.account 与 request.state.tried_accounts。
    """
    auth_header = request.headers.get("Authorization", "")
    if not auth_header.startswith("Bearer "):
        raise HTTPException(
            status_code=401, detail="Unauthorized: missing Bearer token."
        )
    caller_key = auth_header.replace("Bearer ", "", 1).strip()
    config_keys = CONFIG.get("keys", [])
    if caller_key in config_keys:
        request.state.use_config_token = True
        request.state.tried_accounts = []  # 初始化已尝试账号
        selected_account = choose_new_account()
        if not selected_account:
            raise HTTPException(
                status_code=429,
                detail="No accounts configured or all accounts are busy.",
            )
        if not selected_account.get("token", "").strip():
            try:
                login_deepseek_via_account(selected_account)
            except Exception as e:
                logger.error(
                    f"[determine_mode_and_token] 账号 {get_account_identifier(selected_account)} 登录失败：{e}"
                )
                raise HTTPException(status_code=500, detail="Account login failed.")

        request.state.deepseek_token = selected_account.get("token")
        request.state.account = selected_account

    else:
        request.state.use_config_token = False
        request.state.deepseek_token = caller_key


def get_auth_headers(request: Request) -> dict:
    """返回 DeepSeek 请求所需的公共请求头"""
    return {**BASE_HEADERS, "authorization": f"Bearer {request.state.deepseek_token}"}


# ----------------------------------------------------------------------
# Claude 认证相关函数
# ----------------------------------------------------------------------
def determine_claude_mode_and_token(request: Request):
    """Claude认证：沿用现有的OpenAI接口认证逻辑"""
    determine_mode_and_token(request)


# ----------------------------------------------------------------------
# Token 刷新机制
# ----------------------------------------------------------------------
def refresh_account_token(request: Request) -> bool:
    """当 token 过期时，刷新账号 token。
    
    返回 True 表示刷新成功，False 表示刷新失败。
    调用后 request.state.deepseek_token 会被更新。
    """
    if not getattr(request.state, 'use_config_token', False):
        # 用户自带 token，无法刷新
        return False
    
    account = getattr(request.state, 'account', None)
    if not account:
        return False
    
    acc_id = get_account_identifier(account)
    logger.info(f"[refresh_account_token] 尝试刷新账号 {acc_id} 的 token")
    
    try:
        # 清除旧 token
        account["token"] = ""
        # 重新登录
        login_deepseek_via_account(account)
        # 更新 request 状态
        request.state.deepseek_token = account.get("token")
        logger.info(f"[refresh_account_token] 账号 {acc_id} token 刷新成功")
        return True
    except Exception as e:
        logger.error(f"[refresh_account_token] 账号 {acc_id} token 刷新失败: {e}")
        return False


def mark_token_invalid(request: Request):
    """标记当前账号的 token 为无效，清除它以便下次重新登录"""
    if not getattr(request.state, 'use_config_token', False):
        return
    
    account = getattr(request.state, 'account', None)
    if account:
        acc_id = get_account_identifier(account)
        logger.warning(f"[mark_token_invalid] 标记账号 {acc_id} 的 token 为无效")
        account["token"] = ""

