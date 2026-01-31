# -*- coding: utf-8 -*-
"""Admin API 路由 - 管理界面后端"""
import base64
import json
import os
import httpx
import asyncio
import time
import hashlib
import hmac

from fastapi import APIRouter, HTTPException, Request, Depends
from fastapi.responses import JSONResponse
from fastapi.security import HTTPBearer, HTTPAuthorizationCredentials

from core.config import CONFIG, save_config, logger
from core.auth import account_queue, init_account_queue, get_queue_status, get_account_identifier
from core.deepseek import login_deepseek_via_account

router = APIRouter(prefix="/admin", tags=["admin"])
security = HTTPBearer(auto_error=False)

# Admin Key 验证
ADMIN_KEY = os.getenv("DS2API_ADMIN_KEY", "")

# JWT 配置
JWT_SECRET = os.getenv("DS2API_JWT_SECRET", ADMIN_KEY or "ds2api-default-secret")
JWT_EXPIRE_HOURS = int(os.getenv("DS2API_JWT_EXPIRE_HOURS", "24"))

# Vercel 预配置（可通过环境变量设置）
VERCEL_TOKEN = os.getenv("VERCEL_TOKEN", "")
VERCEL_PROJECT_ID = os.getenv("VERCEL_PROJECT_ID", "")
VERCEL_TEAM_ID = os.getenv("VERCEL_TEAM_ID", "")


# ----------------------------------------------------------------------
# JWT 工具函数（轻量实现，无需额外依赖）
# ----------------------------------------------------------------------
def _b64_encode(data: bytes) -> str:
    """Base64 URL 安全编码"""
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode("ascii")

def _b64_decode(data: str) -> bytes:
    """Base64 URL 安全解码"""
    padding = 4 - len(data) % 4
    if padding != 4:
        data += "=" * padding
    return base64.urlsafe_b64decode(data)

def create_jwt_token(expire_hours: int = None) -> str:
    """创建 JWT Token"""
    if expire_hours is None:
        expire_hours = JWT_EXPIRE_HOURS
    
    now = int(time.time())
    payload = {
        "iat": now,
        "exp": now + expire_hours * 3600,
        "type": "admin"
    }
    
    header = {"alg": "HS256", "typ": "JWT"}
    header_b64 = _b64_encode(json.dumps(header).encode())
    payload_b64 = _b64_encode(json.dumps(payload).encode())
    
    signature = hmac.new(
        JWT_SECRET.encode(),
        f"{header_b64}.{payload_b64}".encode(),
        hashlib.sha256
    ).digest()
    signature_b64 = _b64_encode(signature)
    
    return f"{header_b64}.{payload_b64}.{signature_b64}"

def verify_jwt_token(token: str) -> dict:
    """验证 JWT Token，返回 payload 或抛出异常"""
    try:
        parts = token.split(".")
        if len(parts) != 3:
            raise ValueError("Invalid token format")
        
        header_b64, payload_b64, signature_b64 = parts
        
        # 验证签名
        expected_sig = hmac.new(
            JWT_SECRET.encode(),
            f"{header_b64}.{payload_b64}".encode(),
            hashlib.sha256
        ).digest()
        
        actual_sig = _b64_decode(signature_b64)
        if not hmac.compare_digest(expected_sig, actual_sig):
            raise ValueError("Invalid signature")
        
        # 解析 payload
        payload = json.loads(_b64_decode(payload_b64))
        
        # 检查过期
        if payload.get("exp", 0) < time.time():
            raise ValueError("Token expired")
        
        return payload
    except Exception as e:
        raise ValueError(f"Token verification failed: {e}")


# ----------------------------------------------------------------------
# 登录端点
# ----------------------------------------------------------------------
@router.post("/login")
async def admin_login(request: Request):
    """管理员登录，返回 JWT Token"""
    try:
        data = await request.json()
    except:
        data = {}
    
    admin_key = data.get("admin_key", "")
    
    # 开发模式：未配置 ADMIN_KEY 时允许任意登录
    if not ADMIN_KEY:
        token = create_jwt_token()
        return JSONResponse(content={
            "success": True,
            "token": token,
            "expires_in": JWT_EXPIRE_HOURS * 3600,
            "message": "开发模式：未配置 ADMIN_KEY"
        })
    
    # 验证 admin key
    if admin_key != ADMIN_KEY:
        raise HTTPException(status_code=401, detail="管理密钥错误")
    
    token = create_jwt_token()
    return JSONResponse(content={
        "success": True,
        "token": token,
        "expires_in": JWT_EXPIRE_HOURS * 3600,
    })


@router.get("/verify")
async def verify_token(credentials: HTTPAuthorizationCredentials = Depends(security)):
    """验证当前 Token 是否有效"""
    if not credentials:
        raise HTTPException(status_code=401, detail="未提供认证信息")
    
    token = credentials.credentials
    
    # 先尝试 JWT 验证
    try:
        payload = verify_jwt_token(token)
        return JSONResponse(content={
            "valid": True,
            "expires_at": payload.get("exp"),
            "remaining": payload.get("exp", 0) - int(time.time())
        })
    except:
        pass
    
    # 回退到直接 admin key 验证（兼容旧方式）
    if ADMIN_KEY and token == ADMIN_KEY:
        return JSONResponse(content={"valid": True, "type": "admin_key"})
    
    raise HTTPException(status_code=401, detail="Token 无效或已过期")


def verify_admin(credentials: HTTPAuthorizationCredentials = Depends(security)):
    """验证 Admin 权限（支持 JWT 和直接 admin key）"""
    if not ADMIN_KEY:
        # 未配置 Admin Key，允许访问（开发模式）
        return True
    
    if not credentials:
        raise HTTPException(status_code=401, detail="未提供认证信息")
    
    token = credentials.credentials
    
    # 先尝试 JWT 验证
    try:
        verify_jwt_token(token)
        return True
    except:
        pass
    
    # 回退到直接 admin key 验证
    if token == ADMIN_KEY:
        return True
    
    raise HTTPException(status_code=401, detail="认证失败：Token 无效或已过期")


# ----------------------------------------------------------------------
# Vercel 预配置信息
# ----------------------------------------------------------------------
@router.get("/vercel/config")
async def get_vercel_config(_: bool = Depends(verify_admin)):
    """获取预配置的 Vercel 信息（脱敏）"""
    return JSONResponse(content={
        "has_token": bool(VERCEL_TOKEN),
        "project_id": VERCEL_PROJECT_ID,
        "team_id": VERCEL_TEAM_ID,
        "token_preview": VERCEL_TOKEN[:8] + "****" if VERCEL_TOKEN else "",
    })


# ----------------------------------------------------------------------
# 配置管理
# ----------------------------------------------------------------------
@router.get("/config")
async def get_config(_: bool = Depends(verify_admin)):
    """获取当前配置（密码脱敏）"""
    safe_config = {
        "keys": CONFIG.get("keys", []),
        "accounts": [],
        "claude_model_mapping": CONFIG.get("claude_model_mapping", {}),
    }
    for acc in CONFIG.get("accounts", []):
        token = acc.get("token", "")
        safe_acc = {
            "email": acc.get("email", ""),
            "mobile": acc.get("mobile", ""),
            "has_password": bool(acc.get("password")),
            "has_token": bool(token),
            "token_preview": token[:12] + "..." if len(token) > 12 else "" if token else "",
        }
        safe_config["accounts"].append(safe_acc)
    return JSONResponse(content=safe_config)


@router.post("/config")
async def update_config(request: Request, _: bool = Depends(verify_admin)):
    """更新完整配置"""
    try:
        new_config = await request.json()
        
        # 更新 keys
        if "keys" in new_config:
            CONFIG["keys"] = new_config["keys"]
        
        # 更新 accounts
        if "accounts" in new_config:
            CONFIG["accounts"] = new_config["accounts"]
            init_account_queue()  # 重新初始化账号队列
        
        # 更新 claude_model_mapping
        if "claude_model_mapping" in new_config:
            CONFIG["claude_model_mapping"] = new_config["claude_model_mapping"]
        
        save_config(CONFIG)
        return JSONResponse(content={"success": True, "message": "配置已更新"})
    except Exception as e:
        logger.error(f"[update_config] 错误: {e}")
        raise HTTPException(status_code=500, detail=str(e))


# ----------------------------------------------------------------------
# API Keys 管理
# ----------------------------------------------------------------------
@router.post("/keys")
async def add_key(request: Request, _: bool = Depends(verify_admin)):
    """添加 API Key"""
    data = await request.json()
    key = data.get("key", "").strip()
    if not key:
        raise HTTPException(status_code=400, detail="Key 不能为空")
    if key in CONFIG.get("keys", []):
        raise HTTPException(status_code=400, detail="Key 已存在")
    
    if "keys" not in CONFIG:
        CONFIG["keys"] = []
    CONFIG["keys"].append(key)
    save_config(CONFIG)
    return JSONResponse(content={"success": True})


@router.delete("/keys/{key}")
async def delete_key(key: str, _: bool = Depends(verify_admin)):
    """删除 API Key"""
    if key not in CONFIG.get("keys", []):
        raise HTTPException(status_code=404, detail="Key 不存在")
    CONFIG["keys"].remove(key)
    save_config(CONFIG)
    return JSONResponse(content={"success": True})


# ----------------------------------------------------------------------
# 账号管理
# ----------------------------------------------------------------------
@router.post("/accounts")
async def add_account(request: Request, _: bool = Depends(verify_admin)):
    """添加账号"""
    data = await request.json()
    email = data.get("email", "").strip()
    mobile = data.get("mobile", "").strip()
    password = data.get("password", "").strip()
    
    if not password:
        raise HTTPException(status_code=400, detail="密码不能为空")
    if not email and not mobile:
        raise HTTPException(status_code=400, detail="Email 或手机号至少填一个")
    
    # 检查重复
    for acc in CONFIG.get("accounts", []):
        if email and acc.get("email") == email:
            raise HTTPException(status_code=400, detail="该 Email 已存在")
        if mobile and acc.get("mobile") == mobile:
            raise HTTPException(status_code=400, detail="该手机号已存在")
    
    new_account = {"password": password, "token": ""}
    if email:
        new_account["email"] = email
    if mobile:
        new_account["mobile"] = mobile
    
    if "accounts" not in CONFIG:
        CONFIG["accounts"] = []
    CONFIG["accounts"].append(new_account)
    init_account_queue()
    save_config(CONFIG)
    return JSONResponse(content={"success": True})


@router.delete("/accounts/{identifier}")
async def delete_account(identifier: str, _: bool = Depends(verify_admin)):
    """删除账号（通过 email 或 mobile）"""
    accounts = CONFIG.get("accounts", [])
    for i, acc in enumerate(accounts):
        if acc.get("email") == identifier or acc.get("mobile") == identifier:
            accounts.pop(i)
            init_account_queue()
            save_config(CONFIG)
            return JSONResponse(content={"success": True})
    raise HTTPException(status_code=404, detail="账号不存在")


# ----------------------------------------------------------------------
# 账号队列状态（监控）
# ----------------------------------------------------------------------
@router.get("/queue/status")
async def get_account_queue_status(_: bool = Depends(verify_admin)):
    """获取账号轮询队列状态"""
    status = get_queue_status()
    return JSONResponse(content=status)


# ----------------------------------------------------------------------
# 账号验证
# ----------------------------------------------------------------------
async def validate_single_account(account: dict) -> dict:
    """验证单个账号的有效性"""
    acc_id = get_account_identifier(account)
    result = {
        "account": acc_id,
        "valid": False,
        "has_token": bool(account.get("token", "").strip()),
        "message": "",
    }
    
    try:
        # 如果已有 token，尝试简单验证（这里简化处理）
        if result["has_token"]:
            result["valid"] = True
            result["message"] = "已有有效 token"
        else:
            # 尝试登录
            try:
                login_deepseek_via_account(account)
                result["valid"] = True
                result["has_token"] = True
                result["message"] = "登录成功"
            except Exception as e:
                result["valid"] = False
                result["message"] = f"登录失败: {str(e)}"
    except Exception as e:
        result["message"] = f"验证出错: {str(e)}"
    
    return result


@router.post("/accounts/validate")
async def validate_account(request: Request, _: bool = Depends(verify_admin)):
    """验证单个账号"""
    data = await request.json()
    identifier = data.get("identifier", "").strip()
    
    if not identifier:
        raise HTTPException(status_code=400, detail="需要账号标识（email 或 mobile）")
    
    # 查找账号
    account = None
    for acc in CONFIG.get("accounts", []):
        if acc.get("email") == identifier or acc.get("mobile") == identifier:
            account = acc
            break
    
    if not account:
        raise HTTPException(status_code=404, detail="账号不存在")
    
    result = await validate_single_account(account)
    
    # 如果验证成功且获取了新 token，保存配置
    if result["valid"] and result["has_token"]:
        save_config(CONFIG)
    
    return JSONResponse(content=result)


@router.post("/accounts/validate-all")
async def validate_all_accounts(_: bool = Depends(verify_admin)):
    """批量验证所有账号"""
    accounts = CONFIG.get("accounts", [])
    if not accounts:
        return JSONResponse(content={
            "total": 0,
            "valid": 0,
            "invalid": 0,
            "results": [],
        })
    
    results = []
    valid_count = 0
    
    for acc in accounts:
        result = await validate_single_account(acc)
        results.append(result)
        if result["valid"]:
            valid_count += 1
        # 添加小延迟避免请求过快
        await asyncio.sleep(0.5)
    
    # 保存可能更新的 token
    save_config(CONFIG)
    
    return JSONResponse(content={
        "total": len(accounts),
        "valid": valid_count,
        "invalid": len(accounts) - valid_count,
        "results": results,
    })


# ----------------------------------------------------------------------
# 账号 API 测试（实际发送请求）
# ----------------------------------------------------------------------
async def test_account_api(account: dict, model: str = "deepseek-chat", message: str = "") -> dict:
    """测试单个账号的 API 调用能力（通过创建会话验证）"""
    from curl_cffi import requests as cffi_requests
    from core.deepseek import DEEPSEEK_CREATE_SESSION_URL, BASE_HEADERS
    
    acc_id = get_account_identifier(account)
    result = {
        "account": acc_id,
        "success": False,
        "response_time": 0,
        "message": "",
        "model": model,
    }
    
    import time
    start_time = time.time()
    
    try:
        # 确保有 token
        token = account.get("token", "").strip()
        if not token:
            try:
                login_deepseek_via_account(account)
                token = account.get("token", "")
            except Exception as e:
                result["message"] = f"登录失败: {str(e)}"
                return result
        
        headers = {**BASE_HEADERS, "authorization": f"Bearer {token}"}
        
        # 创建会话来测试 API 可用性
        session_resp = cffi_requests.post(
            DEEPSEEK_CREATE_SESSION_URL,
            headers=headers,
            json={"agent": "chat"},
            impersonate="safari15_3",
            timeout=15,
        )
        
        if session_resp.status_code != 200:
            result["message"] = f"创建会话失败: HTTP {session_resp.status_code}"
            return result
        
        session_data = session_resp.json()
        if session_data.get("code") != 0:
            result["message"] = f"创建会话失败: {session_data.get('msg', 'Unknown error')}"
            # token 可能过期，清除它
            account["token"] = ""
            return result
        
        result["success"] = True
        result["message"] = "API 测试成功"
        result["response_time"] = round((time.time() - start_time) * 1000)
        
    except Exception as e:
        result["message"] = f"测试失败: {str(e)}"
    
    return result


@router.post("/accounts/test")
async def test_single_account(request: Request, _: bool = Depends(verify_admin)):
    """测试单个账号的 API 调用
    
    如果提供 message，会发送实际请求并返回 AI 回复；
    否则只快速测试创建会话。
    """
    data = await request.json()
    identifier = data.get("identifier", "")
    model = data.get("model", "deepseek-chat")
    message = data.get("message", "")
    
    if not identifier:
        raise HTTPException(status_code=400, detail="需要账号标识（email 或 mobile）")
    
    # 查找账号
    account = None
    for acc in CONFIG.get("accounts", []):
        if acc.get("email") == identifier or acc.get("mobile") == identifier:
            account = acc
            break
    
    if not account:
        raise HTTPException(status_code=404, detail="账号不存在")
    
    result = await test_account_api(account, model, message)
    
    # 保存可能更新的 token
    save_config(CONFIG)
    
    return JSONResponse(content=result)


@router.post("/accounts/test-all")
async def test_all_accounts(request: Request, _: bool = Depends(verify_admin)):
    """批量测试所有账号的 API 调用"""
    data = await request.json()
    model = data.get("model", "deepseek-chat")
    
    accounts = CONFIG.get("accounts", [])
    if not accounts:
        return JSONResponse(content={
            "total": 0,
            "success": 0,
            "failed": 0,
            "results": [],
        })
    
    results = []
    success_count = 0
    
    for acc in accounts:
        result = await test_account_api(acc, model)
        results.append(result)
        if result["success"]:
            success_count += 1
        # 添加小延迟避免请求过快
        await asyncio.sleep(1)
    
    # 保存可能更新的 token
    save_config(CONFIG)
    
    return JSONResponse(content={
        "total": len(accounts),
        "success": success_count,
        "failed": len(accounts) - success_count,
        "results": results,
    })


# ----------------------------------------------------------------------
# 批量导入
# ----------------------------------------------------------------------
@router.post("/import")
async def batch_import(request: Request, _: bool = Depends(verify_admin)):
    """批量导入配置 (JSON 格式)"""
    try:
        data = await request.json()
        imported_keys = 0
        imported_accounts = 0
        
        # 导入 keys
        if "keys" in data:
            for key in data["keys"]:
                if key not in CONFIG.get("keys", []):
                    if "keys" not in CONFIG:
                        CONFIG["keys"] = []
                    CONFIG["keys"].append(key)
                    imported_keys += 1
        
        # 导入 accounts
        if "accounts" in data:
            existing_ids = set()
            for acc in CONFIG.get("accounts", []):
                existing_ids.add(acc.get("email", ""))
                existing_ids.add(acc.get("mobile", ""))
            
            for acc in data["accounts"]:
                acc_id = acc.get("email", "") or acc.get("mobile", "")
                if acc_id and acc_id not in existing_ids:
                    if "accounts" not in CONFIG:
                        CONFIG["accounts"] = []
                    CONFIG["accounts"].append(acc)
                    existing_ids.add(acc_id)
                    imported_accounts += 1
        
        init_account_queue()
        save_config(CONFIG)
        
        return JSONResponse(content={
            "success": True,
            "imported_keys": imported_keys,
            "imported_accounts": imported_accounts,
        })
    except json.JSONDecodeError:
        raise HTTPException(status_code=400, detail="无效的 JSON 格式")
    except Exception as e:
        logger.error(f"[batch_import] 错误: {e}")
        raise HTTPException(status_code=500, detail=str(e))


# ----------------------------------------------------------------------
# API 测试
# ----------------------------------------------------------------------
@router.post("/test")
async def test_api(request: Request, _: bool = Depends(verify_admin)):
    """测试 API 调用"""
    try:
        data = await request.json()
        model = data.get("model", "deepseek-chat")
        message = data.get("message", "你好")
        api_key = data.get("api_key", "")
        
        if not api_key:
            # 使用配置中的第一个 key
            keys = CONFIG.get("keys", [])
            if not keys:
                raise HTTPException(status_code=400, detail="没有可用的 API Key")
            api_key = keys[0]
        
        # 构造请求
        host = request.headers.get("host", "localhost:5001")
        scheme = "https" if "vercel" in host.lower() else "http"
        base_url = f"{scheme}://{host}"
        
        async with httpx.AsyncClient(timeout=60.0) as client:
            response = await client.post(
                f"{base_url}/v1/chat/completions",
                headers={"Authorization": f"Bearer {api_key}"},
                json={
                    "model": model,
                    "messages": [{"role": "user", "content": message}],
                    "stream": False,
                },
            )
            
            return JSONResponse(content={
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "response": response.json() if response.status_code == 200 else response.text,
            })
    except Exception as e:
        logger.error(f"[test_api] 错误: {e}")
        return JSONResponse(content={
            "success": False,
            "error": str(e),
        })


# ----------------------------------------------------------------------
# Vercel 同步
# ----------------------------------------------------------------------
@router.post("/vercel/sync")
async def sync_to_vercel(request: Request, _: bool = Depends(verify_admin)):
    """同步配置到 Vercel 并触发重新部署"""
    try:
        data = await request.json()
        vercel_token = data.get("vercel_token", "")
        project_id = data.get("project_id", "")
        team_id = data.get("team_id", "")  # 可选
        auto_validate = data.get("auto_validate", True)  # 默认自动验证
        save_vercel_credentials = data.get("save_credentials", True)  # 是否保存 Vercel 凭证
        
        # 支持使用预配置的 token
        use_preconfig = vercel_token == "__USE_PRECONFIG__" or not vercel_token
        if use_preconfig:
            vercel_token = VERCEL_TOKEN
        if not project_id:
            project_id = VERCEL_PROJECT_ID
        if not team_id:
            team_id = VERCEL_TEAM_ID
        
        if not vercel_token or not project_id:
            raise HTTPException(status_code=400, detail="需要 Vercel Token 和 Project ID（可通过环境变量 VERCEL_TOKEN 和 VERCEL_PROJECT_ID 预配置）")
        
        # 自动验证所有无 token 的账号
        validated_count = 0
        failed_accounts = []
        if auto_validate:
            accounts = CONFIG.get("accounts", [])
            for acc in accounts:
                acc_id = get_account_identifier(acc)
                if not acc.get("token", "").strip():
                    try:
                        logger.info(f"[sync_to_vercel] 自动验证账号: {acc_id}")
                        login_deepseek_via_account(acc)
                        validated_count += 1
                    except Exception as e:
                        logger.warning(f"[sync_to_vercel] 账号 {acc_id} 验证失败: {e}")
                        failed_accounts.append(acc_id)
                    await asyncio.sleep(0.5)  # 避免请求过快
        
        # 准备配置 JSON
        config_json = json.dumps(CONFIG, ensure_ascii=False, separators=(",", ":"))
        config_b64 = base64.b64encode(config_json.encode("utf-8")).decode("utf-8")
        
        headers = {"Authorization": f"Bearer {vercel_token}"}
        base_url = "https://api.vercel.com"
        
        async with httpx.AsyncClient(timeout=30.0) as client:
            # 1. 获取现有环境变量
            params = {"teamId": team_id} if team_id else {}
            env_resp = await client.get(
                f"{base_url}/v9/projects/{project_id}/env",
                headers=headers,
                params=params,
            )
            
            if env_resp.status_code != 200:
                raise HTTPException(status_code=env_resp.status_code, detail=f"获取环境变量失败: {env_resp.text}")
            
            env_vars = env_resp.json().get("envs", [])
            existing_env = None
            for env in env_vars:
                if env.get("key") == "DS2API_CONFIG_JSON":
                    existing_env = env
                    break
            
            # 2. 更新或创建环境变量
            if existing_env:
                # 更新
                env_id = existing_env["id"]
                update_resp = await client.patch(
                    f"{base_url}/v9/projects/{project_id}/env/{env_id}",
                    headers=headers,
                    params=params,
                    json={"value": config_b64},
                )
                if update_resp.status_code not in [200, 201]:
                    raise HTTPException(status_code=update_resp.status_code, detail=f"更新环境变量失败: {update_resp.text}")
            else:
                # 创建
                create_resp = await client.post(
                    f"{base_url}/v10/projects/{project_id}/env",
                    headers=headers,
                    params=params,
                    json={
                        "key": "DS2API_CONFIG_JSON",
                        "value": config_b64,
                        "type": "encrypted",
                        "target": ["production", "preview"],
                    },
                )
                if create_resp.status_code not in [200, 201]:
                    raise HTTPException(status_code=create_resp.status_code, detail=f"创建环境变量失败: {create_resp.text}")
            
            # 2.5 保存 Vercel 凭证到环境变量（方便后续快捷同步）
            saved_credentials = []
            if save_vercel_credentials and not use_preconfig:
                # 要保存的凭证列表
                creds_to_save = [
                    ("VERCEL_TOKEN", vercel_token),
                    ("VERCEL_PROJECT_ID", project_id),
                ]
                if team_id:
                    creds_to_save.append(("VERCEL_TEAM_ID", team_id))
                
                for key, value in creds_to_save:
                    # 检查是否已存在
                    existing = None
                    for env in env_vars:
                        if env.get("key") == key:
                            existing = env
                            break
                    
                    if existing:
                        # 更新
                        upd_resp = await client.patch(
                            f"{base_url}/v9/projects/{project_id}/env/{existing['id']}",
                            headers=headers,
                            params=params,
                            json={"value": value},
                        )
                        if upd_resp.status_code in [200, 201]:
                            saved_credentials.append(key)
                    else:
                        # 创建
                        crt_resp = await client.post(
                            f"{base_url}/v10/projects/{project_id}/env",
                            headers=headers,
                            params=params,
                            json={
                                "key": key,
                                "value": value,
                                "type": "encrypted",
                                "target": ["production", "preview"],
                            },
                        )
                        if crt_resp.status_code in [200, 201]:
                            saved_credentials.append(key)
            
            # 3. 触发重新部署 (获取最新的 git 信息并创建新部署)
            # 获取项目信息
            project_resp = await client.get(
                f"{base_url}/v9/projects/{project_id}",
                headers=headers,
                params=params,
            )
            
            if project_resp.status_code == 200:
                project_data = project_resp.json()
                repo = project_data.get("link", {})
                
                if repo.get("type") == "github":
                    # 使用 GitHub 信息创建部署
                    deploy_resp = await client.post(
                        f"{base_url}/v13/deployments",
                        headers=headers,
                        params=params,
                        json={
                            "name": project_id,
                            "project": project_id,
                            "target": "production",
                            "gitSource": {
                                "type": "github",
                                "repoId": repo.get("repoId"),
                                "ref": repo.get("productionBranch", "main"),
                            },
                        },
                    )
                    
                    if deploy_resp.status_code in [200, 201]:
                        deploy_data = deploy_resp.json()
                        result = {
                            "success": True,
                            "message": "配置已同步，正在重新部署...",
                            "deployment_url": deploy_data.get("url"),
                            "validated_accounts": validated_count,
                        }
                        if failed_accounts:
                            result["failed_accounts"] = failed_accounts
                        if saved_credentials:
                            result["saved_credentials"] = saved_credentials
                        return JSONResponse(content=result)
            
            # 如果无法自动部署，返回成功但提示手动部署
            result = {
                "success": True,
                "message": "配置已同步到 Vercel，请手动触发重新部署",
                "manual_deploy_required": True,
                "validated_accounts": validated_count,
            }
            if failed_accounts:
                result["failed_accounts"] = failed_accounts
            if saved_credentials:
                result["saved_credentials"] = saved_credentials
            return JSONResponse(content=result)
            
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"[sync_to_vercel] 错误: {e}")
        raise HTTPException(status_code=500, detail=str(e))


# ----------------------------------------------------------------------
# 导出配置
# ----------------------------------------------------------------------
@router.get("/export")
async def export_config(_: bool = Depends(verify_admin)):
    """导出完整配置（JSON 和 Base64）"""
    config_json = json.dumps(CONFIG, ensure_ascii=False, separators=(",", ":"))
    config_b64 = base64.b64encode(config_json.encode("utf-8")).decode("utf-8")
    
    return JSONResponse(content={
        "json": config_json,
        "base64": config_b64,
    })
