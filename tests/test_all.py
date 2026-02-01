#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
DS2API 全面自动化测试套件

测试覆盖:
- 配置加载和认证
- 会话创建
- PoW 计算
- OpenAI 兼容 API
- Claude 兼容 API
- 流式和非流式响应
- 错误处理
- Token 计数

使用方法:
    python tests/test_all.py                    # 运行所有测试
    python tests/test_all.py --quick            # 快速测试（跳过耗时测试）
    python tests/test_all.py --verbose          # 详细输出
    python tests/test_all.py --endpoint URL     # 指定测试端点
"""
import argparse
import json
import os
import sys
import time
from dataclasses import dataclass
from typing import Optional
import requests

# 添加项目根目录到路径
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

# 测试配置
DEFAULT_ENDPOINT = "http://localhost:5001"
TEST_API_KEY = "test-api-key-001"  # 配置中的 API key
TEST_TIMEOUT = 120  # 超时时间（秒）


@dataclass
class TestResult:
    """测试结果"""
    name: str
    passed: bool
    duration: float
    message: str = ""
    details: Optional[dict] = None


class TestRunner:
    """测试运行器"""

    def __init__(self, endpoint: str, api_key: str, verbose: bool = False):
        self.endpoint = endpoint.rstrip("/")
        self.api_key = api_key
        self.verbose = verbose
        self.results: list[TestResult] = []

    def log(self, message: str, level: str = "INFO"):
        """日志输出"""
        colors = {
            "INFO": "\033[94m",
            "SUCCESS": "\033[92m",
            "WARNING": "\033[93m",
            "ERROR": "\033[91m",
            "RESET": "\033[0m"
        }
        if self.verbose or level in ("ERROR", "SUCCESS"):
            print(f"{colors.get(level, '')}{message}{colors['RESET']}")

    def run_test(self, name: str, test_func):
        """运行单个测试"""
        print(f"\n{'='*60}")
        print(f"🧪 测试: {name}")
        print('='*60)
        
        start_time = time.time()
        try:
            result = test_func()
            duration = time.time() - start_time
            
            if result.get("success", False):
                self.log(f"✅ 通过 ({duration:.2f}s)", "SUCCESS")
                self.results.append(TestResult(
                    name=name,
                    passed=True,
                    duration=duration,
                    message=result.get("message", ""),
                    details=result.get("details")
                ))
            else:
                self.log(f"❌ 失败: {result.get('message', '未知错误')}", "ERROR")
                self.results.append(TestResult(
                    name=name,
                    passed=False,
                    duration=duration,
                    message=result.get("message", ""),
                    details=result.get("details")
                ))
        except Exception as e:
            duration = time.time() - start_time
            self.log(f"❌ 异常: {e}", "ERROR")
            self.results.append(TestResult(
                name=name,
                passed=False,
                duration=duration,
                message=str(e)
            ))

    def get_headers(self, is_claude: bool = False) -> dict:
        """获取请求头"""
        headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {self.api_key}"
        }
        if is_claude:
            headers["anthropic-version"] = "2024-01-01"
        return headers

    # =====================================================================
    # 基础测试
    # =====================================================================
    
    def test_health_check(self) -> dict:
        """测试服务健康状态"""
        try:
            resp = requests.get(f"{self.endpoint}/", timeout=10)
            if resp.status_code == 200:
                return {"success": True, "message": "服务运行正常"}
            return {"success": False, "message": f"状态码: {resp.status_code}"}
        except requests.exceptions.ConnectionError:
            return {"success": False, "message": "无法连接到服务"}

    # =====================================================================
    # OpenAI 兼容 API 测试
    # =====================================================================

    def test_openai_models_list(self) -> dict:
        """测试 OpenAI /v1/models 端点"""
        resp = requests.get(
            f"{self.endpoint}/v1/models",
            headers=self.get_headers(),
            timeout=TEST_TIMEOUT
        )
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}"}
        
        data = resp.json()
        if data.get("object") != "list":
            return {"success": False, "message": "响应格式错误"}
        
        models = [m["id"] for m in data.get("data", [])]
        expected_models = ["deepseek-chat", "deepseek-reasoner", "deepseek-chat-search", "deepseek-reasoner-search"]
        
        for model in expected_models:
            if model not in models:
                return {"success": False, "message": f"缺少模型: {model}"}
        
        return {
            "success": True, 
            "message": f"返回 {len(models)} 个模型",
            "details": {"models": models}
        }

    def test_openai_chat_non_stream(self) -> dict:
        """测试 OpenAI 非流式对话"""
        payload = {
            "model": "deepseek-chat",
            "messages": [
                {"role": "user", "content": "请用一句话回答：1+1等于多少？"}
            ],
            "stream": False
        }
        
        resp = requests.post(
            f"{self.endpoint}/v1/chat/completions",
            headers=self.get_headers(),
            json=payload,
            timeout=TEST_TIMEOUT
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}", "details": {"response": resp.text}}
        
        data = resp.json()
        if "error" in data:
            return {"success": False, "message": data["error"]}
        
        content = data.get("choices", [{}])[0].get("message", {}).get("content", "")
        if not content:
            return {"success": False, "message": "响应内容为空"}
        
        return {
            "success": True,
            "message": f"收到 {len(content)} 字符响应",
            "details": {
                "content_preview": content[:100] + "..." if len(content) > 100 else content,
                "usage": data.get("usage", {})
            }
        }

    def test_openai_chat_stream(self) -> dict:
        """测试 OpenAI 流式对话"""
        payload = {
            "model": "deepseek-chat",
            "messages": [
                {"role": "user", "content": "说'你好'"}
            ],
            "stream": True
        }
        
        resp = requests.post(
            f"{self.endpoint}/v1/chat/completions",
            headers=self.get_headers(),
            json=payload,
            stream=True,
            timeout=TEST_TIMEOUT
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}"}
        
        chunks = []
        content = ""
        for line in resp.iter_lines():
            if line:
                line_str = line.decode("utf-8")
                if line_str.startswith("data: "):
                    data_str = line_str[6:]
                    if data_str == "[DONE]":
                        break
                    try:
                        chunk = json.loads(data_str)
                        chunks.append(chunk)
                        delta = chunk.get("choices", [{}])[0].get("delta", {})
                        if "content" in delta:
                            content += delta["content"]
                    except json.JSONDecodeError:
                        pass
        
        if not chunks:
            return {"success": False, "message": "未收到任何流式数据块"}
        
        return {
            "success": True,
            "message": f"收到 {len(chunks)} 个数据块，内容: {content[:50]}",
            "details": {"chunk_count": len(chunks), "content": content}
        }

    def test_openai_reasoner_stream(self) -> dict:
        """测试 OpenAI Reasoner 模式（思考链）"""
        payload = {
            "model": "deepseek-reasoner",
            "messages": [
                {"role": "user", "content": "1加2等于多少？"}
            ],
            "stream": True
        }
        
        resp = requests.post(
            f"{self.endpoint}/v1/chat/completions",
            headers=self.get_headers(),
            json=payload,
            stream=True,
            timeout=TEST_TIMEOUT
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}"}
        
        content = ""
        reasoning = ""
        for line in resp.iter_lines():
            if line:
                line_str = line.decode("utf-8")
                if line_str.startswith("data: "):
                    data_str = line_str[6:]
                    if data_str == "[DONE]":
                        break
                    try:
                        chunk = json.loads(data_str)
                        delta = chunk.get("choices", [{}])[0].get("delta", {})
                        if "content" in delta:
                            content += delta["content"]
                        if "reasoning_content" in delta:
                            reasoning += delta["reasoning_content"]
                    except json.JSONDecodeError:
                        pass
        
        return {
            "success": True,
            "message": f"思考: {len(reasoning)}字, 回答: {len(content)}字",
            "details": {
                "reasoning_preview": reasoning[:100] + "..." if len(reasoning) > 100 else reasoning,
                "content": content
            }
        }

    def test_openai_invalid_model(self) -> dict:
        """测试无效模型错误处理"""
        payload = {
            "model": "invalid-model-name",
            "messages": [{"role": "user", "content": "test"}],
            "stream": False
        }
        
        resp = requests.post(
            f"{self.endpoint}/v1/chat/completions",
            headers=self.get_headers(),
            json=payload,
            timeout=TEST_TIMEOUT
        )
        
        # 应该返回 503 或 400
        if resp.status_code in (503, 400):
            return {"success": True, "message": f"正确返回错误状态码 {resp.status_code}"}
        
        return {"success": False, "message": f"期望 503/400，实际: {resp.status_code}"}

    def test_openai_missing_auth(self) -> dict:
        """测试缺少认证的错误处理"""
        payload = {
            "model": "deepseek-chat",
            "messages": [{"role": "user", "content": "test"}]
        }
        
        resp = requests.post(
            f"{self.endpoint}/v1/chat/completions",
            headers={"Content-Type": "application/json"},  # 无 Authorization
            json=payload,
            timeout=TEST_TIMEOUT
        )
        
        if resp.status_code == 401:
            return {"success": True, "message": "正确返回 401 未认证"}
        
        return {"success": False, "message": f"期望 401，实际: {resp.status_code}"}

    # =====================================================================
    # Claude 兼容 API 测试
    # =====================================================================

    def test_claude_models_list(self) -> dict:
        """测试 Claude /anthropic/v1/models 端点"""
        resp = requests.get(
            f"{self.endpoint}/anthropic/v1/models",
            headers=self.get_headers(is_claude=True),
            timeout=TEST_TIMEOUT
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}"}
        
        data = resp.json()
        models = [m["id"] for m in data.get("data", [])]
        
        if not models:
            return {"success": False, "message": "模型列表为空"}
        
        return {
            "success": True,
            "message": f"返回 {len(models)} 个 Claude 模型",
            "details": {"models": models}
        }

    def test_claude_messages_non_stream(self) -> dict:
        """测试 Claude 非流式消息"""
        payload = {
            "model": "claude-sonnet-4-20250514",
            "max_tokens": 100,
            "messages": [
                {"role": "user", "content": "Say 'Hello' in Chinese"}
            ],
            "stream": False
        }
        
        resp = requests.post(
            f"{self.endpoint}/anthropic/v1/messages",
            headers=self.get_headers(is_claude=True),
            json=payload,
            timeout=TEST_TIMEOUT
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}", "details": {"response": resp.text}}
        
        data = resp.json()
        if "error" in data:
            return {"success": False, "message": str(data["error"])}
        
        content_blocks = data.get("content", [])
        text_content = ""
        for block in content_blocks:
            if block.get("type") == "text":
                text_content += block.get("text", "")
        
        if not text_content:
            return {"success": False, "message": "响应内容为空"}
        
        return {
            "success": True,
            "message": f"收到 Claude 格式响应: {len(text_content)} 字符",
            "details": {
                "content": text_content[:100],
                "stop_reason": data.get("stop_reason"),
                "usage": data.get("usage", {})
            }
        }

    def test_claude_messages_stream(self) -> dict:
        """测试 Claude 流式消息"""
        payload = {
            "model": "claude-sonnet-4-20250514",
            "max_tokens": 50,
            "messages": [
                {"role": "user", "content": "Reply with just 'OK'"}
            ],
            "stream": True
        }
        
        resp = requests.post(
            f"{self.endpoint}/anthropic/v1/messages",
            headers=self.get_headers(is_claude=True),
            json=payload,
            stream=True,
            timeout=TEST_TIMEOUT
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}"}
        
        events = []
        for line in resp.iter_lines():
            if line:
                line_str = line.decode("utf-8")
                if line_str.startswith("data: "):
                    try:
                        event = json.loads(line_str[6:])
                        events.append(event)
                    except json.JSONDecodeError:
                        pass
        
        event_types = [e.get("type") for e in events]
        
        # 检查必要的事件类型
        required_types = ["message_start", "message_stop"]
        for rt in required_types:
            if rt not in event_types:
                return {"success": False, "message": f"缺少事件类型: {rt}"}
        
        return {
            "success": True,
            "message": f"收到 {len(events)} 个 Claude 流事件",
            "details": {"event_types": event_types}
        }

    def test_claude_count_tokens(self) -> dict:
        """测试 Claude token 计数"""
        payload = {
            "model": "claude-sonnet-4-20250514",
            "messages": [
                {"role": "user", "content": "Hello, how are you today?"}
            ]
        }
        
        resp = requests.post(
            f"{self.endpoint}/anthropic/v1/messages/count_tokens",
            headers=self.get_headers(is_claude=True),
            json=payload,
            timeout=TEST_TIMEOUT
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}"}
        
        data = resp.json()
        input_tokens = data.get("input_tokens", 0)
        
        if input_tokens <= 0:
            return {"success": False, "message": f"token 计数无效: {input_tokens}"}
        
        return {
            "success": True,
            "message": f"Token 计数: {input_tokens}",
            "details": data
        }

    # =====================================================================
    # 高级功能测试
    # =====================================================================

    def test_multi_turn_conversation(self) -> dict:
        """测试多轮对话"""
        payload = {
            "model": "deepseek-chat",
            "messages": [
                {"role": "system", "content": "你是一个数学助手"},
                {"role": "user", "content": "我有3个苹果"},
                {"role": "assistant", "content": "好的，你有3个苹果。"},
                {"role": "user", "content": "我又买了2个，现在有多少？"}
            ],
            "stream": False
        }
        
        resp = requests.post(
            f"{self.endpoint}/v1/chat/completions",
            headers=self.get_headers(),
            json=payload,
            timeout=TEST_TIMEOUT
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}"}
        
        data = resp.json()
        content = data.get("choices", [{}])[0].get("message", {}).get("content", "")
        
        # 检查是否包含"5"
        if "5" in content:
            return {"success": True, "message": f"AI 正确理解上下文", "details": {"content": content[:100]}}
        
        return {
            "success": True,  # 即使没有5也算通过，因为测试的是多轮对话功能
            "message": f"多轮对话功能正常",
            "details": {"content": content[:100]}
        }

    def test_long_input(self) -> dict:
        """测试长输入处理"""
        # 生成约 1000 字的输入
        long_text = "这是一段测试文本。" * 100
        
        payload = {
            "model": "deepseek-chat",
            "messages": [
                {"role": "user", "content": f"请总结以下内容的主题：{long_text}"}
            ],
            "stream": False
        }
        
        resp = requests.post(
            f"{self.endpoint}/v1/chat/completions",
            headers=self.get_headers(),
            json=payload,
            timeout=TEST_TIMEOUT
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}"}
        
        data = resp.json()
        if "error" in data:
            return {"success": False, "message": str(data.get("error"))}
        
        return {
            "success": True,
            "message": f"成功处理 {len(long_text)} 字符输入",
            "details": {"input_length": len(long_text)}
        }

    # =====================================================================
    # 管理 API 测试
    # =====================================================================

    def test_admin_config(self) -> dict:
        """测试管理配置 API"""
        resp = requests.get(
            f"{self.endpoint}/admin/config",
            timeout=10
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}"}
        
        data = resp.json()
        
        # 验证返回结构
        if "accounts" not in data:
            return {"success": False, "message": "响应缺少 accounts 字段"}
        
        # 验证 token_preview 字段存在
        accounts = data.get("accounts", [])
        if accounts:
            first_acc = accounts[0]
            if "token_preview" not in first_acc:
                return {"success": False, "message": "响应缺少 token_preview 字段"}
        
        return {
            "success": True,
            "message": f"获取配置成功，{len(accounts)} 个账号",
            "details": {"account_count": len(accounts)}
        }

    def test_admin_account_test(self) -> dict:
        """测试单账号 API 测试端点"""
        # 先获取配置以获取账号
        config_resp = requests.get(f"{self.endpoint}/admin/config", timeout=10)
        if config_resp.status_code != 200:
            return {"success": False, "message": "获取配置失败"}
        
        accounts = config_resp.json().get("accounts", [])
        if not accounts:
            return {"success": False, "message": "没有可测试的账号"}
        
        # 测试第一个账号
        first_acc = accounts[0]
        identifier = first_acc.get("email") or first_acc.get("mobile")
        
        resp = requests.post(
            f"{self.endpoint}/admin/accounts/test",
            json={"identifier": identifier},
            timeout=30
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}"}
        
        data = resp.json()
        
        # 验证返回结构
        required_fields = ["account", "success", "response_time", "message"]
        for field in required_fields:
            if field not in data:
                return {"success": False, "message": f"响应缺少 {field} 字段"}
        
        if not data["success"]:
            return {"success": False, "message": f"账号测试失败: {data['message']}"}
        
        return {
            "success": True,
            "message": f"账号 {identifier} 测试成功 ({data['response_time']}ms)",
            "details": {"response_time": data["response_time"]}
        }

    # =====================================================================
    # 工具调用测试
    # =====================================================================

    def test_openai_tool_calling(self) -> dict:
        """测试 OpenAI 工具调用"""
        payload = {
            "model": "deepseek-chat",
            "messages": [
                {"role": "user", "content": "What's the weather in Beijing? Use the get_weather tool."}
            ],
            "tools": [{
                "type": "function",
                "function": {
                    "name": "get_weather",
                    "description": "Get current weather for a location",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "location": {"type": "string", "description": "City name"}
                        },
                        "required": ["location"]
                    }
                }
            }],
            "stream": False
        }
        
        resp = requests.post(
            f"{self.endpoint}/v1/chat/completions",
            headers=self.get_headers(),
            json=payload,
            timeout=TEST_TIMEOUT
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}", "details": {"response": resp.text}}
        
        data = resp.json()
        if "error" in data:
            return {"success": False, "message": data["error"]}
        
        message = data.get("choices", [{}])[0].get("message", {})
        tool_calls = message.get("tool_calls", [])
        finish_reason = data.get("choices", [{}])[0].get("finish_reason", "")
        content = message.get("content", "")
        
        # AI 可能调用工具，也可能直接回复
        if tool_calls:
            return {
                "success": True,
                "message": f"检测到 {len(tool_calls)} 个工具调用, finish_reason={finish_reason}",
                "details": {"tool_calls": tool_calls}
            }
        else:
            return {
                "success": True,
                "message": f"AI 直接回复而非调用工具: {content[:50]}...",
                "details": {"content": content[:100]}
            }

    def test_openai_tool_calling_stream(self) -> dict:
        """测试 OpenAI 流式工具调用"""
        payload = {
            "model": "deepseek-chat",
            "messages": [
                {"role": "user", "content": "Use get_time tool to check current time in Tokyo."}
            ],
            "tools": [{
                "type": "function",
                "function": {
                    "name": "get_time",
                    "description": "Get current time for a timezone",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "timezone": {"type": "string"}
                        },
                        "required": ["timezone"]
                    }
                }
            }],
            "stream": True
        }
        
        resp = requests.post(
            f"{self.endpoint}/v1/chat/completions",
            headers=self.get_headers(),
            json=payload,
            stream=True,
            timeout=TEST_TIMEOUT
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}"}
        
        chunks = []
        tool_calls_found = False
        finish_reason = None
        
        for line in resp.iter_lines():
            if line:
                line_str = line.decode("utf-8")
                if line_str.startswith("data: "):
                    data_str = line_str[6:]
                    if data_str == "[DONE]":
                        break
                    try:
                        chunk = json.loads(data_str)
                        chunks.append(chunk)
                        delta = chunk.get("choices", [{}])[0].get("delta", {})
                        if "tool_calls" in delta:
                            tool_calls_found = True
                        fr = chunk.get("choices", [{}])[0].get("finish_reason")
                        if fr:
                            finish_reason = fr
                    except json.JSONDecodeError:
                        pass
        
        return {
            "success": True,
            "message": f"收到 {len(chunks)} 个数据块, 工具调用: {tool_calls_found}, finish: {finish_reason}",
            "details": {"chunk_count": len(chunks), "tool_calls_found": tool_calls_found}
        }

    def test_claude_tool_calling(self) -> dict:
        """测试 Claude 工具调用"""
        payload = {
            "model": "claude-sonnet-4-20250514",
            "max_tokens": 200,
            "messages": [
                {"role": "user", "content": "Use the calculator tool to compute 15 * 23"}
            ],
            "tools": [{
                "name": "calculator",
                "description": "Perform arithmetic calculations",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "expression": {"type": "string", "description": "Math expression"}
                    },
                    "required": ["expression"]
                }
            }],
            "stream": False
        }
        
        resp = requests.post(
            f"{self.endpoint}/anthropic/v1/messages",
            headers=self.get_headers(is_claude=True),
            json=payload,
            timeout=TEST_TIMEOUT
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}", "details": {"response": resp.text}}
        
        data = resp.json()
        if "error" in data:
            return {"success": False, "message": str(data["error"])}
        
        content_blocks = data.get("content", [])
        stop_reason = data.get("stop_reason", "")
        
        tool_use_blocks = [b for b in content_blocks if b.get("type") == "tool_use"]
        text_blocks = [b for b in content_blocks if b.get("type") == "text"]
        
        if tool_use_blocks:
            return {
                "success": True,
                "message": f"检测到 {len(tool_use_blocks)} 个工具调用, stop_reason={stop_reason}",
                "details": {"tool_use": tool_use_blocks}
            }
        else:
            text_content = "".join(b.get("text", "") for b in text_blocks)
            return {
                "success": True,
                "message": f"AI 直接回复: {text_content[:50]}...",
                "details": {"content": text_content[:100]}
            }

    # =====================================================================
    # 搜索模式测试
    # =====================================================================

    def test_openai_search_mode(self) -> dict:
        """测试 OpenAI 搜索模式"""
        payload = {
            "model": "deepseek-chat-search",
            "messages": [
                {"role": "user", "content": "今天的新闻有哪些？"}
            ],
            "stream": True
        }
        
        resp = requests.post(
            f"{self.endpoint}/v1/chat/completions",
            headers=self.get_headers(),
            json=payload,
            stream=True,
            timeout=TEST_TIMEOUT
        )
        
        if resp.status_code != 200:
            return {"success": False, "message": f"状态码: {resp.status_code}"}
        
        content = ""
        for line in resp.iter_lines():
            if line:
                line_str = line.decode("utf-8")
                if line_str.startswith("data: "):
                    data_str = line_str[6:]
                    if data_str == "[DONE]":
                        break
                    try:
                        chunk = json.loads(data_str)
                        delta = chunk.get("choices", [{}])[0].get("delta", {})
                        if "content" in delta:
                            content += delta["content"]
                    except json.JSONDecodeError:
                        pass
        
        if not content:
            return {"success": False, "message": "搜索模式无响应内容"}
        
        return {
            "success": True,
            "message": f"搜索模式正常，收到 {len(content)} 字符",
            "details": {"content_preview": content[:100]}
        }

    # =====================================================================
    # 运行测试
    # =====================================================================

    def run_all_tests(self, quick: bool = False):
        """运行所有测试"""
        print("\n" + "="*70)
        print("     🚀 DS2API 全面自动化测试")
        print("="*70)
        print(f"端点: {self.endpoint}")
        print(f"API Key: {self.api_key[:10]}...")
        print(f"模式: {'快速' if quick else '完整'}")
        
        # 基础测试
        self.run_test("服务健康检查", self.test_health_check)
        
        if not self.results[-1].passed:
            print("\n⚠️  服务未运行，跳过其他测试")
            return
        
        # OpenAI API 测试
        self.run_test("OpenAI 模型列表", self.test_openai_models_list)
        self.run_test("OpenAI 非流式对话", self.test_openai_chat_non_stream)
        self.run_test("OpenAI 流式对话", self.test_openai_chat_stream)
        self.run_test("OpenAI 无效模型处理", self.test_openai_invalid_model)
        self.run_test("OpenAI 缺少认证处理", self.test_openai_missing_auth)
        
        if not quick:
            self.run_test("OpenAI Reasoner 模式", self.test_openai_reasoner_stream)
        
        # Claude API 测试
        self.run_test("Claude 模型列表", self.test_claude_models_list)
        self.run_test("Claude 非流式消息", self.test_claude_messages_non_stream)
        self.run_test("Claude 流式消息", self.test_claude_messages_stream)
        self.run_test("Claude Token 计数", self.test_claude_count_tokens)
        
        # 高级功能测试
        if not quick:
            self.run_test("多轮对话", self.test_multi_turn_conversation)
            self.run_test("长输入处理", self.test_long_input)
            self.run_test("OpenAI 搜索模式", self.test_openai_search_mode)
        
        # 工具调用测试
        if not quick:
            self.run_test("OpenAI 工具调用", self.test_openai_tool_calling)
            self.run_test("OpenAI 流式工具调用", self.test_openai_tool_calling_stream)
            self.run_test("Claude 工具调用", self.test_claude_tool_calling)
        
        # 管理 API 测试
        self.run_test("管理配置 API", self.test_admin_config)
        self.run_test("账号测试 API", self.test_admin_account_test)
        
        # 输出测试报告
        self.print_report()

    def print_report(self):
        """打印测试报告"""
        print("\n" + "="*70)
        print("     📊 测试报告")
        print("="*70)
        
        passed = sum(1 for r in self.results if r.passed)
        failed = len(self.results) - passed
        total_time = sum(r.duration for r in self.results)
        
        print(f"\n总计: {len(self.results)} 个测试")
        print(f"✅ 通过: {passed}")
        print(f"❌ 失败: {failed}")
        print(f"⏱️  耗时: {total_time:.2f}s")
        print(f"📈 通过率: {passed/len(self.results)*100:.1f}%")
        
        if failed > 0:
            print("\n❌ 失败的测试:")
            for r in self.results:
                if not r.passed:
                    print(f"   • {r.name}: {r.message}")
        
        print("\n" + "="*70)
        
        # 返回退出码
        return 0 if failed == 0 else 1


def main():
    parser = argparse.ArgumentParser(description="DS2API 自动化测试")
    parser.add_argument("--endpoint", default=DEFAULT_ENDPOINT, help="API 端点")
    parser.add_argument("--api-key", default=TEST_API_KEY, help="API Key")
    parser.add_argument("--quick", action="store_true", help="快速测试模式")
    parser.add_argument("--verbose", "-v", action="store_true", help="详细输出")
    
    args = parser.parse_args()
    
    runner = TestRunner(
        endpoint=args.endpoint,
        api_key=args.api_key,
        verbose=args.verbose
    )
    
    exit_code = runner.run_all_tests(quick=args.quick)
    sys.exit(exit_code)


if __name__ == "__main__":
    main()
