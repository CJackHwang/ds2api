#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
DS2API 单元测试

测试核心模块的功能，不依赖网络请求
"""
import json
import os
import sys
import unittest

# 添加项目根目录到路径
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


class TestConfig(unittest.TestCase):
    """配置模块测试"""

    def test_config_loading(self):
        """测试配置加载"""
        from core.config import load_config, CONFIG
        
        # 测试加载函数不会抛出异常
        config = load_config()
        self.assertIsInstance(config, dict)

    def test_config_paths(self):
        """测试配置路径"""
        from core.config import WASM_PATH, CONFIG_PATH
        
        # 路径应该是字符串
        self.assertIsInstance(WASM_PATH, str)
        self.assertIsInstance(CONFIG_PATH, str)


class TestMessages(unittest.TestCase):
    """消息处理模块测试"""

    def test_messages_prepare_simple(self):
        """测试简单消息处理"""
        from core.messages import messages_prepare
        
        messages = [
            {"role": "user", "content": "Hello"}
        ]
        result = messages_prepare(messages)
        self.assertIn("Hello", result)

    def test_messages_prepare_multi_turn(self):
        """测试多轮对话消息处理"""
        from core.messages import messages_prepare
        
        messages = [
            {"role": "system", "content": "You are a helper."},
            {"role": "user", "content": "Hi"},
            {"role": "assistant", "content": "Hello!"},
            {"role": "user", "content": "How are you?"}
        ]
        result = messages_prepare(messages)
        
        # 检查助手消息标签
        self.assertIn("<｜Assistant｜>", result)
        self.assertIn("<｜end▁of▁sentence｜>", result)
        # 检查用户消息标签
        self.assertIn("<｜User｜>", result)

    def test_messages_prepare_array_content(self):
        """测试数组格式内容处理"""
        from core.messages import messages_prepare
        
        messages = [
            {
                "role": "user",
                "content": [
                    {"type": "text", "text": "First part"},
                    {"type": "text", "text": "Second part"},
                    {"type": "image", "url": "http://example.com/image.png"}
                ]
            }
        ]
        result = messages_prepare(messages)
        
        self.assertIn("First part", result)
        self.assertIn("Second part", result)

    def test_markdown_image_removal(self):
        """测试 markdown 图片格式移除"""
        from core.messages import messages_prepare
        
        messages = [
            {"role": "user", "content": "Check this ![alt](http://example.com/image.png) image"}
        ]
        result = messages_prepare(messages)
        
        # 图片格式应该被改为链接格式
        self.assertNotIn("![alt]", result)
        self.assertIn("[alt]", result)

    def test_merge_consecutive_messages(self):
        """测试连续相同角色消息合并"""
        from core.messages import messages_prepare
        
        messages = [
            {"role": "user", "content": "Part 1"},
            {"role": "user", "content": "Part 2"},
            {"role": "user", "content": "Part 3"}
        ]
        result = messages_prepare(messages)
        
        self.assertIn("Part 1", result)
        self.assertIn("Part 2", result)
        self.assertIn("Part 3", result)

    def test_convert_claude_to_deepseek(self):
        """测试 Claude 到 DeepSeek 格式转换"""
        from core.messages import convert_claude_to_deepseek
        
        claude_request = {
            "model": "claude-sonnet-4-20250514",
            "messages": [{"role": "user", "content": "Hi"}],
            "system": "You are helpful.",
            "temperature": 0.7,
            "stream": True
        }
        
        result = convert_claude_to_deepseek(claude_request)
        
        # 检查模型映射
        self.assertIn("deepseek", result.get("model", "").lower())
        
        # 检查 system 消息插入
        self.assertEqual(result["messages"][0]["role"], "system")
        self.assertEqual(result["messages"][0]["content"], "You are helpful.")
        
        # 检查其他参数
        self.assertEqual(result.get("temperature"), 0.7)
        self.assertEqual(result.get("stream"), True)


class TestPow(unittest.TestCase):
    """PoW 模块测试"""

    def test_wasm_caching(self):
        """测试 WASM 缓存功能"""
        from core.pow import _get_cached_wasm_module, _wasm_module, _wasm_engine
        from core.config import WASM_PATH
        
        # 首次调用
        engine1, module1 = _get_cached_wasm_module(WASM_PATH)
        self.assertIsNotNone(engine1)
        self.assertIsNotNone(module1)
        
        # 再次调用应该返回相同的实例
        engine2, module2 = _get_cached_wasm_module(WASM_PATH)
        self.assertIs(engine1, engine2)
        self.assertIs(module1, module2)

    def test_get_account_identifier(self):
        """测试账号标识获取"""
        from core.utils import get_account_identifier
        
        # 测试邮箱
        account1 = {"email": "test@example.com"}
        self.assertEqual(get_account_identifier(account1), "test@example.com")
        
        # 测试手机号
        account2 = {"mobile": "13800138000"}
        self.assertEqual(get_account_identifier(account2), "13800138000")
        
        # 邮箱优先
        account3 = {"email": "test@example.com", "mobile": "13800138000"}
        self.assertEqual(get_account_identifier(account3), "test@example.com")


class TestSessionManager(unittest.TestCase):
    """会话管理器模块测试"""

    def test_get_model_config(self):
        """测试模型配置获取"""
        from core.session_manager import get_model_config
        
        # deepseek-chat
        thinking, search = get_model_config("deepseek-chat")
        self.assertEqual(thinking, False)
        self.assertEqual(search, False)
        
        # deepseek-reasoner
        thinking, search = get_model_config("deepseek-reasoner")
        self.assertEqual(thinking, True)
        self.assertEqual(search, False)
        
        # deepseek-chat-search
        thinking, search = get_model_config("deepseek-chat-search")
        self.assertEqual(thinking, False)
        self.assertEqual(search, True)
        
        # deepseek-reasoner-search
        thinking, search = get_model_config("deepseek-reasoner-search")
        self.assertEqual(thinking, True)
        self.assertEqual(search, True)
        
        # 大小写不敏感
        thinking, search = get_model_config("DeepSeek-CHAT")
        self.assertEqual(thinking, False)
        self.assertEqual(search, False)
        
        # 无效模型
        thinking, search = get_model_config("invalid-model")
        self.assertIsNone(thinking)
        self.assertIsNone(search)


class TestAuth(unittest.TestCase):
    """认证模块测试"""

    def test_auth_key_check(self):
        """测试 API Key 检查"""
        from core.config import CONFIG
        
        # 检查配置中是否有 keys
        keys = CONFIG.get("keys", [])
        self.assertIsInstance(keys, list)


class TestRegexPatterns(unittest.TestCase):
    """正则表达式测试"""

    def test_markdown_image_pattern(self):
        """测试 markdown 图片正则"""
        from core.messages import _MARKDOWN_IMAGE_PATTERN
        
        text = "Check ![alt text](http://example.com/image.png) here"
        match = _MARKDOWN_IMAGE_PATTERN.search(text)
        
        self.assertIsNotNone(match)
        self.assertEqual(match.group(1), "alt text")
        self.assertEqual(match.group(2), "http://example.com/image.png")


class TestStreamParsing(unittest.TestCase):
    """流式响应解析测试"""

    def test_parse_simple_string_content(self):
        """测试简单字符串内容解析"""
        # 模拟 DeepSeek V3 的简单字符串格式
        chunk = {"v": "你好"}
        
        v_value = chunk.get("v")
        self.assertIsInstance(v_value, str)
        self.assertEqual(v_value, "你好")

    def test_parse_nested_list_content(self):
        """测试嵌套列表内容解析 (DeepSeek V3 格式)"""
        # 模拟 DeepSeek V3 的嵌套列表格式
        chunk = {
            "p": "response/fragments",
            "o": "APPEND",
            "v": [
                {"id": 1, "type": "RESPONSE", "content": "我是DeepSeek", "references": [], "stage_id": 1}
            ]
        }
        
        v_value = chunk.get("v")
        self.assertIsInstance(v_value, list)
        self.assertEqual(len(v_value), 1)
        
        inner = v_value[0]
        self.assertEqual(inner.get("type"), "RESPONSE")
        self.assertEqual(inner.get("content"), "我是DeepSeek")

    def test_parse_thinking_content(self):
        """测试 thinking 内容解析"""
        # 模拟带有 THINK 类型的内容 (DeepSeek 使用 THINK 而不是 THINKING)
        chunk = {
            "p": "response/fragments",
            "o": "APPEND",
            "v": [
                {"id": 1, "type": "THINK", "content": "让我思考一下...", "references": [], "stage_id": 1}
            ]
        }
        
        v_value = chunk.get("v")
        inner = v_value[0]
        
        inner_type = inner.get("type", "").upper()
        self.assertEqual(inner_type, "THINK")
        self.assertEqual(inner.get("content"), "让我思考一下...")

    def test_parse_finished_status(self):
        """测试 FINISHED 状态解析"""
        chunk = {"p": "response/status", "o": "SET", "v": "FINISHED"}
        
        v_value = chunk.get("v")
        self.assertEqual(v_value, "FINISHED")

    def test_parse_batch_status(self):
        """测试批量状态解析"""
        chunk = {
            "p": "response",
            "o": "BATCH",
            "v": [
                {"p": "accumulated_token_usage", "v": 54},
                {"p": "quasi_status", "v": "FINISHED"}
            ]
        }
        
        v_value = chunk.get("v")
        self.assertIsInstance(v_value, list)
        
        # 检查是否包含 FINISHED 状态
        has_finished = any(
            item.get("p") == "quasi_status" and item.get("v") == "FINISHED"
            for item in v_value if isinstance(item, dict)
        )
        self.assertTrue(has_finished)

    def test_extract_content_from_nested_response(self):
        """测试从嵌套响应中提取内容"""
        # 模拟完整的嵌套列表格式
        items = [
            {"p": "fragments", "o": "APPEND", "v": [
                {"id": 1, "type": "RESPONSE", "content": "Hello", "references": []}
            ]},
            {"p": "search_status", "v": "searching"},  # 应该被跳过
        ]
        
        extracted = []
        for item in items:
            if not isinstance(item, dict):
                continue
            
            item_p = item.get("p", "")
            item_v = item.get("v")
            
            # 跳过搜索状态
            if "search_status" in item_p:
                continue
            
            if isinstance(item_v, list):
                for inner in item_v:
                    if isinstance(inner, dict):
                        content = inner.get("content", "")
                        if content:
                            inner_type = inner.get("type", "").upper()
                            extracted.append((content, inner_type))
        
        self.assertEqual(len(extracted), 1)
        self.assertEqual(extracted[0], ("Hello", "RESPONSE"))

    def test_thinking_vs_text_classification(self):
        """测试 thinking 和 text 类型分类"""
        # 测试不同路径的类型分类
        test_cases = [
            ("response/thinking_content", "thinking"),
            ("response/content", "text"),
            ("response/fragments", "text"),
            ("", "text"),  # 默认类型
        ]
        
        for chunk_path, expected_type in test_cases:
            if chunk_path == "response/thinking_content":
                ptype = "thinking"
            elif chunk_path == "response/content" or "response/fragments" in chunk_path:
                ptype = "text"
            else:
                ptype = "text"
            
            self.assertEqual(ptype, expected_type, f"Path '{chunk_path}' should be '{expected_type}'")

    def test_handle_non_dict_items(self):
        """测试处理非字典类型的列表项"""
        items = [
            "plain string",
            123,
            None,
            {"p": "content", "v": "valid"},
        ]
        
        valid_items = [item for item in items if isinstance(item, dict)]
        self.assertEqual(len(valid_items), 1)
        self.assertEqual(valid_items[0].get("v"), "valid")

    def test_empty_content_handling(self):
        """测试空内容处理"""
        chunk = {"v": ""}
        
        content = chunk.get("v", "")
        # 空内容不应该被添加
        self.assertFalse(bool(content))

    def test_response_started_flag(self):
        """测试 response_started 标志逻辑 - 只有 RESPONSE 类型才触发"""
        response_started = False
        thinking_enabled = True
        
        # 模拟处理流程 - 修复后的逻辑
        chunks = [
            {"v": "思考中..."},  # thinking (before response)
            {"p": "response/fragments", "v": [{"type": "THINK", "content": "思考"}]},  # THINK 不触发 response_started
            {"v": "继续思考..."},  # 仍然是 thinking
            {"p": "response/fragments", "v": [{"type": "RESPONSE", "content": "回复"}]},  # RESPONSE 触发
            {"v": "正式回复"},  # text (after response started)
        ]
        
        results = []
        for chunk in chunks:
            chunk_path = chunk.get("p", "")
            v_value = chunk.get("v")
            
            # 只有当 fragments 包含 RESPONSE 类型时才设置 response_started
            if "response/fragments" in chunk_path and isinstance(v_value, list):
                for frag in v_value:
                    if isinstance(frag, dict) and frag.get("type", "").upper() == "RESPONSE":
                        response_started = True
                        break
            
            if not chunk_path:
                if thinking_enabled and not response_started:
                    ptype = "thinking"
                else:
                    ptype = "text"
            else:
                ptype = "text"
            
            results.append((ptype, response_started))
        
        self.assertEqual(results[0], ("thinking", False))  # 第一个是 thinking
        self.assertEqual(results[1], ("text", False))      # THINK fragment 不触发 response_started
        self.assertEqual(results[2], ("thinking", False))  # THINK 之后仍是 thinking
        self.assertEqual(results[3], ("text", True))       # RESPONSE fragment 触发
        self.assertEqual(results[4], ("text", True))       # 之后是 text

    def test_think_vs_response_fragment_types(self):
        """测试 THINK 和 RESPONSE fragment 类型的区分"""
        # 模拟 DeepSeek 的 fragments 数据
        think_fragment = {"p": "response/fragments", "v": [{"id": 1, "type": "THINK", "content": "嗯"}]}
        response_fragment = {"p": "response/fragments", "v": [{"id": 2, "type": "RESPONSE", "content": "你好"}]}
        
        def check_response_started(chunk):
            """检查是否应该设置 response_started"""
            chunk_path = chunk.get("p", "")
            v_value = chunk.get("v")
            if "response/fragments" in chunk_path and isinstance(v_value, list):
                for frag in v_value:
                    if isinstance(frag, dict) and frag.get("type", "").upper() == "RESPONSE":
                        return True
            return False
        
        self.assertFalse(check_response_started(think_fragment))   # THINK 不触发
        self.assertTrue(check_response_started(response_fragment))  # RESPONSE 触发


class TestToolCallParsing(unittest.TestCase):
    """工具调用解析测试"""

    def test_parse_tool_calls_simple(self):
        """测试简单工具调用解析"""
        from core.sse_parser import parse_tool_calls
        
        response_text = '{"tool_calls": [{"name": "get_weather", "input": {"location": "Beijing"}}]}'
        tools = [{"name": "get_weather"}]
        
        result = parse_tool_calls(response_text, tools)
        
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0]["name"], "get_weather")
        self.assertEqual(result[0]["input"]["location"], "Beijing")

    def test_parse_tool_calls_multiple(self):
        """测试多工具调用解析"""
        from core.sse_parser import parse_tool_calls
        
        response_text = '''{"tool_calls": [
            {"name": "get_weather", "input": {"location": "Beijing"}},
            {"name": "get_time", "input": {"timezone": "Asia/Shanghai"}}
        ]}'''
        tools = [{"name": "get_weather"}, {"name": "get_time"}]
        
        result = parse_tool_calls(response_text, tools)
        
        self.assertEqual(len(result), 2)
        self.assertEqual(result[0]["name"], "get_weather")
        self.assertEqual(result[1]["name"], "get_time")

    def test_parse_tool_calls_no_match(self):
        """测试无工具调用时返回空列表"""
        from core.sse_parser import parse_tool_calls
        
        response_text = "这是一个普通的回复，没有工具调用。"
        tools = [{"name": "get_weather"}]
        
        result = parse_tool_calls(response_text, tools)
        
        self.assertEqual(result, [])

    def test_parse_tool_calls_with_surrounding_text(self):
        """测试带有周围文本的工具调用"""
        from core.sse_parser import parse_tool_calls
        
        response_text = '''好的，我来帮你查询天气。
{"tool_calls": [{"name": "get_weather", "input": {"location": "Shanghai"}}]}'''
        tools = [{"name": "get_weather"}]
        
        result = parse_tool_calls(response_text, tools)
        
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0]["name"], "get_weather")

    def test_parse_tool_calls_empty_input(self):
        """测试空输入"""
        from core.sse_parser import parse_tool_calls
        
        result = parse_tool_calls("", [])
        self.assertEqual(result, [])
        
        result = parse_tool_calls("some text", [])
        self.assertEqual(result, [])

    def test_parse_tool_calls_invalid_json(self):
        """测试无效 JSON"""
        from core.sse_parser import parse_tool_calls
        
        response_text = '{"tool_calls": [{"name": "get_weather", invalid json here}'
        tools = [{"name": "get_weather"}]
        
        result = parse_tool_calls(response_text, tools)
        
        # 应该返回空列表而不是抛出异常
        self.assertEqual(result, [])


class TestTokenEstimation(unittest.TestCase):
    """Token 估算测试"""
    
    def test_estimate_tokens_string(self):
        """测试字符串 token 估算"""
        from core.utils import estimate_tokens
        
        # 8个字符应该约等于2个token
        result = estimate_tokens("12345678")
        self.assertEqual(result, 2)
        
        # 空字符串应该返回1
        result = estimate_tokens("")
        self.assertEqual(result, 1)

    def test_estimate_tokens_list(self):
        """测试列表 token 估算"""
        from core.utils import estimate_tokens
        
        content = [
            {"text": "Hello"},
            {"text": "World"}
        ]
        result = estimate_tokens(content)
        self.assertGreater(result, 0)


if __name__ == "__main__":
    # 设置环境变量避免配置警告
    os.environ.setdefault("DS2API_CONFIG_PATH", 
                          os.path.join(os.path.dirname(os.path.dirname(__file__)), "config.json"))
    
    unittest.main(verbosity=2)
