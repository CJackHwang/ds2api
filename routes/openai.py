# -*- coding: utf-8 -*-
"""OpenAI 兼容路由"""
import json
import queue
import random
import re
import threading
import time

from curl_cffi import requests as cffi_requests
from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import JSONResponse, StreamingResponse

from core.config import CONFIG, logger
from core.auth import (
    determine_mode_and_token,
    get_auth_headers,
    release_account,
)
from core.deepseek import call_completion_endpoint
from core.session_manager import (
    create_session,
    get_pow,
    cleanup_account,
)
from core.models import get_model_config, get_openai_models_response
from core.sse_parser import (
    parse_deepseek_sse_line,
    parse_sse_chunk_for_content,
    extract_content_from_chunk,
    extract_content_recursive,
    should_filter_citation,
    parse_tool_calls,
)
from core.constants import (
    KEEP_ALIVE_TIMEOUT,
    STREAM_IDLE_TIMEOUT,
    MAX_KEEPALIVE_COUNT,
)
from core.messages import messages_prepare

router = APIRouter()

# 预编译正则表达式（性能优化）
_CITATION_PATTERN = re.compile(r"^\[citation:")






# ----------------------------------------------------------------------
# 路由：/v1/models
# ----------------------------------------------------------------------
@router.get("/v1/models")
def list_models():
    data = get_openai_models_response()
    return JSONResponse(content=data, status_code=200)


# ----------------------------------------------------------------------
# 路由：/v1/chat/completions
# ----------------------------------------------------------------------
@router.post("/v1/chat/completions")
async def chat_completions(request: Request):
    try:
        # 处理 token 相关逻辑，若登录失败则直接返回错误响应
        try:
            determine_mode_and_token(request)
        except HTTPException as exc:
            return JSONResponse(
                status_code=exc.status_code, content={"error": exc.detail}
            )
        except Exception as exc:
            logger.error(f"[chat_completions] determine_mode_and_token 异常: {exc}")
            return JSONResponse(
                status_code=500, content={"error": "Account login failed."}
            )

        req_data = await request.json()
        model = req_data.get("model")
        messages = req_data.get("messages", [])
        if not model or not messages:
            raise HTTPException(
                status_code=400, detail="Request must include 'model' and 'messages'."
            )
        
        # 解析工具调用参数（OpenAI 格式）
        tools_requested = req_data.get("tools") or []
        has_tools = len(tools_requested) > 0
        
        # 如果有工具定义，构建工具提示并注入到消息中
        messages_with_tools = messages.copy()
        if has_tools:
            tool_schemas = []
            for tool in tools_requested:
                # OpenAI 格式: {"type": "function", "function": {"name": ..., "description": ..., "parameters": ...}}
                func = tool.get("function", tool)  # 兼容简化格式
                tool_name = func.get("name", "unknown")
                tool_desc = func.get("description", "No description available")
                schema = func.get("parameters", {})
                
                tool_info = f"Tool: {tool_name}\nDescription: {tool_desc}"
                if "properties" in schema:
                    props = []
                    required = schema.get("required", [])
                    for prop_name, prop_info in schema["properties"].items():
                        prop_type = prop_info.get("type", "string")
                        is_req = " (required)" if prop_name in required else ""
                        props.append(f"  - {prop_name}: {prop_type}{is_req}")
                    if props:
                        tool_info += f"\nParameters:\n" + "\n".join(props)
                tool_schemas.append(tool_info)
            
            # 检查是否已有系统消息
            has_system = any(m.get("role") == "system" for m in messages_with_tools)
            tool_prompt = f"""You have access to these tools:

{chr(10).join(tool_schemas)}

When you need to use tools, output ONLY this JSON format (no other text):
{{"tool_calls": [
  {{"name": "tool_name", "input": {{"param": "value"}}}}
]}}

IMPORTANT: If calling tools, output ONLY the JSON. The response must start with {{ and end with }}"""
            
            if has_system:
                # 追加到现有系统消息
                for i, m in enumerate(messages_with_tools):
                    if m.get("role") == "system":
                        messages_with_tools[i] = {
                            "role": "system",
                            "content": m.get("content", "") + "\n\n" + tool_prompt
                        }
                        break
            else:
                # 添加新的系统消息
                messages_with_tools.insert(0, {"role": "system", "content": tool_prompt})
        
        # 使用会话管理器获取模型配置
        thinking_enabled, search_enabled = get_model_config(model)
        if thinking_enabled is None:
            raise HTTPException(
                status_code=503, detail=f"Model '{model}' is not available."
            )
        
        # 使用 messages_prepare 函数构造最终 prompt（使用带工具提示的消息）
        final_prompt = messages_prepare(messages_with_tools)
        session_id = create_session(request)
        if not session_id:
            raise HTTPException(status_code=401, detail="invalid token.")
        
        pow_resp = get_pow(request)
        if not pow_resp:
            raise HTTPException(
                status_code=401,
                detail="Failed to get PoW (invalid token or unknown error).",
            )
        
        headers = {**get_auth_headers(request), "x-ds-pow-response": pow_resp}
        payload = {
            "chat_session_id": session_id,
            "parent_message_id": None,
            "prompt": final_prompt,
            "ref_file_ids": [],
            "thinking_enabled": thinking_enabled,
            "search_enabled": search_enabled,
        }

        deepseek_resp = call_completion_endpoint(payload, headers, max_attempts=3)
        if not deepseek_resp:
            raise HTTPException(status_code=500, detail="Failed to get completion.")
        created_time = int(time.time())
        completion_id = f"{session_id}"

        # 流式响应（SSE）或普通响应
        if bool(req_data.get("stream", False)):
            if deepseek_resp.status_code != 200:
                deepseek_resp.close()
                return JSONResponse(
                    content=deepseek_resp.content, status_code=deepseek_resp.status_code
                )

            def sse_stream():
                # 使用导入的常量（不再本地定义）
                try:
                    final_text = ""
                    final_thinking = ""
                    first_chunk_sent = False
                    result_queue = queue.Queue()
                    last_send_time = time.time()
                    last_content_time = time.time()  # 最后收到有效内容的时间
                    keepalive_count = 0  # 连续 keepalive 计数
                    has_content = False  # 是否收到过内容

                    def process_data():
                        """处理 DeepSeek SSE 数据流 - 使用 sse_parser 模块"""
                        nonlocal has_content
                        current_fragment_type = "thinking" if thinking_enabled else "text"
                        logger.info(f"[sse_stream] 开始处理数据流, session_id={session_id}")
                        
                        try:
                            for raw_line in deepseek_resp.iter_lines():
                                # 解码行
                                try:
                                    line = raw_line.decode("utf-8")
                                except Exception as e:
                                    logger.warning(f"[sse_stream] 解码失败: {e}")
                                    result_queue.put({"choices": [{"index": 0, "delta": {"content": "解码失败，请稍候再试", "type": "text"}}]})
                                    result_queue.put(None)
                                    break
                                
                                if not line:
                                    continue
                                    
                                if not line.startswith("data:"):
                                    continue
                                    
                                data_str = line[5:].strip()
                                if data_str == "[DONE]":
                                    result_queue.put(None)
                                    break
                                    
                                try:
                                    chunk = json.loads(data_str)
                                    
                                    # 检测内容审核/敏感词阻止
                                    if "error" in chunk or chunk.get("code") == "content_filter":
                                        logger.warning(f"[sse_stream] 检测到内容过滤: {chunk}")
                                        result_queue.put({"choices": [{"index": 0, "finish_reason": "content_filter"}]})
                                        result_queue.put(None)
                                        return
                                    
                                    # 使用 sse_parser 模块解析内容
                                    contents, is_finished, new_fragment_type = parse_sse_chunk_for_content(
                                        chunk, thinking_enabled, current_fragment_type
                                    )
                                    current_fragment_type = new_fragment_type
                                    
                                    if is_finished:
                                        result_queue.put({"choices": [{"index": 0, "finish_reason": "stop"}]})
                                        result_queue.put(None)
                                        return
                                    
                                    # 处理提取的内容
                                    for content_text, content_type in contents:
                                        if content_text:
                                            has_content = True
                                            unified_chunk = {
                                                "choices": [{
                                                    "index": 0,
                                                    "delta": {"content": content_text, "type": content_type}
                                                }],
                                                "model": "",
                                                "chunk_token_usage": len(content_text) // 4,
                                                "created": 0,
                                                "message_id": -1,
                                                "parent_id": -1
                                            }
                                            result_queue.put(unified_chunk)
                                            
                                except Exception as e:
                                    logger.warning(f"[sse_stream] 无法解析: {data_str[:100]}, 错误: {e}")
                                    result_queue.put({"choices": [{"index": 0, "delta": {"content": "解析失败，请稍候再试", "type": "text"}}]})
                                    result_queue.put(None)
                                    break
                                    
                        except Exception as e:
                            logger.warning(f"[sse_stream] 错误: {e}")
                            result_queue.put({"choices": [{"index": 0, "delta": {"content": "服务器错误，请稍候再试", "type": "text"}}]})
                            result_queue.put(None)
                        finally:
                            deepseek_resp.close()


                    process_thread = threading.Thread(target=process_data)
                    process_thread.start()

                    while True:
                        current_time = time.time()
                        
                        # 智能超时检测：如果已有内容且长时间无新数据，强制结束
                        if has_content and (current_time - last_content_time) > STREAM_IDLE_TIMEOUT:
                            logger.warning(f"[sse_stream] 智能超时: 已有内容但 {STREAM_IDLE_TIMEOUT}s 无新数据，强制结束")
                            break
                        
                        # 连续 keepalive 检测：如果已有内容且连续多次 keepalive，强制结束
                        if has_content and keepalive_count >= MAX_KEEPALIVE_COUNT:
                            logger.warning(f"[sse_stream] 智能超时: 连续 {MAX_KEEPALIVE_COUNT} 次 keepalive，强制结束")
                            break
                        
                        if current_time - last_send_time >= KEEP_ALIVE_TIMEOUT:
                            yield ": keep-alive\n\n"
                            last_send_time = current_time
                            keepalive_count += 1
                            continue
                            
                        try:
                            chunk = result_queue.get(timeout=0.05)
                            keepalive_count = 0  # 重置 keepalive 计数
                            
                            if chunk is None:
                                prompt_tokens = len(final_prompt) // 4
                                thinking_tokens = len(final_thinking) // 4
                                completion_tokens = len(final_text) // 4
                                usage = {
                                    "prompt_tokens": prompt_tokens,
                                    "completion_tokens": thinking_tokens + completion_tokens,
                                    "total_tokens": prompt_tokens + thinking_tokens + completion_tokens,
                                    "completion_tokens_details": {"reasoning_tokens": thinking_tokens},
                                }
                                
                                # 检测工具调用
                                detected_tools = []
                                finish_reason = "stop"
                                if has_tools:
                                    detected_tools = parse_tool_calls(final_text, [{"name": t.get("function", t).get("name")} for t in tools_requested])
                                    if detected_tools:
                                        finish_reason = "tool_calls"
                                
                                if detected_tools:
                                    # 发送工具调用响应
                                    tool_calls_data = []
                                    for idx, tool_info in enumerate(detected_tools):
                                        tool_calls_data.append({
                                            "id": f"call_{int(time.time())}_{random.randint(1000,9999)}_{idx}",
                                            "type": "function",
                                            "function": {
                                                "name": tool_info["name"],
                                                "arguments": json.dumps(tool_info["input"], ensure_ascii=False)
                                            }
                                        })
                                    tool_chunk = {
                                        "id": completion_id,
                                        "object": "chat.completion.chunk",
                                        "created": created_time,
                                        "model": model,
                                        "choices": [{"delta": {"tool_calls": tool_calls_data}, "index": 0}],
                                    }
                                    yield f"data: {json.dumps(tool_chunk, ensure_ascii=False)}\n\n"
                                
                                finish_chunk = {
                                    "id": completion_id,
                                    "object": "chat.completion.chunk",
                                    "created": created_time,
                                    "model": model,
                                    "choices": [{"delta": {}, "index": 0, "finish_reason": finish_reason}],
                                    "usage": usage,
                                }
                                yield f"data: {json.dumps(finish_chunk, ensure_ascii=False)}\n\n"
                                yield "data: [DONE]\n\n"
                                last_send_time = current_time
                                break
                                
                            new_choices = []
                            for choice in chunk.get("choices", []):
                                delta = choice.get("delta", {})
                                ctype = delta.get("type")
                                ctext = delta.get("content", "")
                                if choice.get("finish_reason") == "backend_busy":
                                    ctext = "服务器繁忙，请稍候再试"
                                if choice.get("finish_reason") == "content_filter":
                                    # 内容过滤，正常结束
                                    pass
                                if search_enabled and ctext.startswith("[citation:"):
                                    ctext = ""
                                if ctype == "thinking":
                                    if thinking_enabled:
                                        final_thinking += ctext
                                else:
                                    # 非 thinking 内容都作为普通文本处理（包括 ctype=None 或 "text"）
                                    final_text += ctext
                                delta_obj = {}
                                if not first_chunk_sent:
                                    delta_obj["role"] = "assistant"
                                    first_chunk_sent = True
                                if ctype == "thinking":
                                    if thinking_enabled:
                                        delta_obj["reasoning_content"] = ctext
                                else:
                                    # 非 thinking 内容都作为 content 输出
                                    if ctext:
                                        delta_obj["content"] = ctext
                                if delta_obj:
                                    new_choices.append({"delta": delta_obj, "index": choice.get("index", 0)})
                                    
                            if new_choices:
                                last_content_time = current_time  # 更新最后内容时间
                                out_chunk = {
                                    "id": completion_id,
                                    "object": "chat.completion.chunk",
                                    "created": created_time,
                                    "model": model,
                                    "choices": new_choices,
                                }
                                yield f"data: {json.dumps(out_chunk, ensure_ascii=False)}\n\n"
                                last_send_time = current_time
                        except queue.Empty:
                            continue
                            
                    # 如果是超时退出，也发送结束标记
                    if has_content:
                        prompt_tokens = len(final_prompt) // 4
                        thinking_tokens = len(final_thinking) // 4
                        completion_tokens = len(final_text) // 4
                        usage = {
                            "prompt_tokens": prompt_tokens,
                            "completion_tokens": thinking_tokens + completion_tokens,
                            "total_tokens": prompt_tokens + thinking_tokens + completion_tokens,
                            "completion_tokens_details": {"reasoning_tokens": thinking_tokens},
                        }
                        
                        # 检测工具调用
                        detected_tools = []
                        finish_reason = "stop"
                        if has_tools:
                            detected_tools = parse_tool_calls(final_text, [{"name": t.get("function", t).get("name")} for t in tools_requested])
                            if detected_tools:
                                finish_reason = "tool_calls"
                        
                        if detected_tools:
                            tool_calls_data = []
                            for idx, tool_info in enumerate(detected_tools):
                                tool_calls_data.append({
                                    "id": f"call_{int(time.time())}_{random.randint(1000,9999)}_{idx}",
                                    "type": "function",
                                    "function": {
                                        "name": tool_info["name"],
                                        "arguments": json.dumps(tool_info["input"], ensure_ascii=False)
                                    }
                                })
                            tool_chunk = {
                                "id": completion_id,
                                "object": "chat.completion.chunk",
                                "created": created_time,
                                "model": model,
                                "choices": [{"delta": {"tool_calls": tool_calls_data}, "index": 0}],
                            }
                            yield f"data: {json.dumps(tool_chunk, ensure_ascii=False)}\n\n"
                        
                        finish_chunk = {
                            "id": completion_id,
                            "object": "chat.completion.chunk",
                            "created": created_time,
                            "model": model,
                            "choices": [{"delta": {}, "index": 0, "finish_reason": finish_reason}],
                            "usage": usage,
                        }
                        yield f"data: {json.dumps(finish_chunk, ensure_ascii=False)}\n\n"
                        yield "data: [DONE]\n\n"
                        
                except Exception as e:
                    logger.error(f"[sse_stream] 异常: {e}")
                finally:
                    cleanup_account(request)

            return StreamingResponse(
                sse_stream(),
                media_type="text/event-stream",
                headers={"Content-Type": "text/event-stream"},
            )
        else:
            # 非流式响应处理
            think_list = []
            text_list = []
            result = None

            data_queue = queue.Queue()

            def collect_data():
                nonlocal result
                ptype = "text"
                try:
                    for raw_line in deepseek_resp.iter_lines():
                        try:
                            line = raw_line.decode("utf-8")
                        except Exception as e:
                            logger.warning(f"[chat_completions] 解码失败: {e}")
                            if ptype == "thinking":
                                think_list.append("解码失败，请稍候再试")
                            else:
                                text_list.append("解码失败，请稍候再试")
                            data_queue.put(None)
                            break
                        if not line:
                            continue
                        if line.startswith("data:"):
                            data_str = line[5:].strip()
                            if data_str == "[DONE]":
                                data_queue.put(None)
                                break
                            try:
                                chunk = json.loads(data_str)
                                if "v" in chunk:
                                    v_value = chunk["v"]
                                    if "p" in chunk and chunk.get("p") == "response/search_status":
                                        continue
                                    if "p" in chunk and chunk.get("p") == "response/thinking_content":
                                        ptype = "thinking"
                                    elif "p" in chunk and chunk.get("p") == "response/content":
                                        ptype = "text"
                                    if isinstance(v_value, str):
                                        if search_enabled and v_value.startswith("[citation:"):
                                            continue
                                        if ptype == "thinking":
                                            think_list.append(v_value)
                                        else:
                                            text_list.append(v_value)
                                    elif isinstance(v_value, list):
                                        for item in v_value:
                                            if item.get("p") == "status" and item.get("v") == "FINISHED":
                                                final_reasoning = "".join(think_list)
                                                final_content = "".join(text_list)
                                                prompt_tokens = len(final_prompt) // 4
                                                reasoning_tokens = len(final_reasoning) // 4
                                                completion_tokens = len(final_content) // 4
                                                
                                                # 检测工具调用
                                                detected_tools = []
                                                finish_reason = "stop"
                                                if has_tools:
                                                    detected_tools = parse_tool_calls(final_content, [{"name": t.get("function", t).get("name")} for t in tools_requested])
                                                    if detected_tools:
                                                        finish_reason = "tool_calls"
                                                
                                                # 构建 message 对象
                                                message_obj = {
                                                    "role": "assistant",
                                                    "content": final_content if not detected_tools else None,
                                                }
                                                # 只有启用思考模式时才包含 reasoning_content
                                                if thinking_enabled and final_reasoning:
                                                    message_obj["reasoning_content"] = final_reasoning
                                                # 添加工具调用
                                                if detected_tools:
                                                    tool_calls_data = []
                                                    for idx, tool_info in enumerate(detected_tools):
                                                        tool_calls_data.append({
                                                            "id": f"call_{int(time.time())}_{random.randint(1000,9999)}_{idx}",
                                                            "type": "function",
                                                            "function": {
                                                                "name": tool_info["name"],
                                                                "arguments": json.dumps(tool_info["input"], ensure_ascii=False)
                                                            }
                                                        })
                                                    message_obj["tool_calls"] = tool_calls_data
                                                    message_obj["content"] = None
                                                
                                                result = {
                                                    "id": completion_id,
                                                    "object": "chat.completion",
                                                    "created": created_time,
                                                    "model": model,
                                                    "choices": [{
                                                        "index": 0,
                                                        "message": message_obj,
                                                        "finish_reason": finish_reason,
                                                    }],
                                                    "usage": {
                                                        "prompt_tokens": prompt_tokens,
                                                        "completion_tokens": reasoning_tokens + completion_tokens,
                                                        "total_tokens": prompt_tokens + reasoning_tokens + completion_tokens,
                                                        "completion_tokens_details": {"reasoning_tokens": reasoning_tokens},
                                                    },
                                                }
                                                data_queue.put("DONE")
                                                return
                            except Exception as e:
                                logger.warning(f"[collect_data] 无法解析: {data_str}, 错误: {e}")
                                if ptype == "thinking":
                                    think_list.append("解析失败，请稍候再试")
                                else:
                                    text_list.append("解析失败，请稍候再试")
                                data_queue.put(None)
                                break
                except Exception as e:
                    logger.warning(f"[collect_data] 错误: {e}")
                    if ptype == "thinking":
                        think_list.append("处理失败，请稍候再试")
                    else:
                        text_list.append("处理失败，请稍候再试")
                    data_queue.put(None)
                finally:
                    deepseek_resp.close()
                    if result is None:
                        final_content = "".join(text_list)
                        final_reasoning = "".join(think_list)
                        prompt_tokens = len(final_prompt) // 4
                        reasoning_tokens = len(final_reasoning) // 4
                        completion_tokens = len(final_content) // 4
                        
                        # 检测工具调用
                        detected_tools = []
                        finish_reason = "stop"
                        if has_tools:
                            detected_tools = parse_tool_calls(final_content, [{"name": t.get("function", t).get("name")} for t in tools_requested])
                            if detected_tools:
                                finish_reason = "tool_calls"
                        
                        # 构建 message 对象
                        message_obj = {
                            "role": "assistant",
                            "content": final_content if not detected_tools else None,
                        }
                        # 只有启用思考模式时才包含 reasoning_content
                        if thinking_enabled and final_reasoning:
                            message_obj["reasoning_content"] = final_reasoning
                        # 添加工具调用
                        if detected_tools:
                            tool_calls_data = []
                            for idx, tool_info in enumerate(detected_tools):
                                tool_calls_data.append({
                                    "id": f"call_{int(time.time())}_{random.randint(1000,9999)}_{idx}",
                                    "type": "function",
                                    "function": {
                                        "name": tool_info["name"],
                                        "arguments": json.dumps(tool_info["input"], ensure_ascii=False)
                                    }
                                })
                            message_obj["tool_calls"] = tool_calls_data
                            message_obj["content"] = None
                        
                        result = {
                            "id": completion_id,
                            "object": "chat.completion",
                            "created": created_time,
                            "model": model,
                            "choices": [{
                                "index": 0,
                                "message": message_obj,
                                "finish_reason": finish_reason,
                            }],
                            "usage": {
                                "prompt_tokens": prompt_tokens,
                                "completion_tokens": reasoning_tokens + completion_tokens,
                                "total_tokens": prompt_tokens + reasoning_tokens + completion_tokens,
                            },
                        }
                    data_queue.put("DONE")

            collect_thread = threading.Thread(target=collect_data)
            collect_thread.start()

            def generate():
                last_send_time = time.time()
                while True:
                    current_time = time.time()
                    if current_time - last_send_time >= KEEP_ALIVE_TIMEOUT:
                        yield ""
                        last_send_time = current_time
                    if not collect_thread.is_alive() and result is not None:
                        yield json.dumps(result)
                        break
                    time.sleep(0.1)

            return StreamingResponse(generate(), media_type="application/json")
    except HTTPException as exc:
        return JSONResponse(status_code=exc.status_code, content={"error": exc.detail})
    except Exception as exc:
        logger.error(f"[chat_completions] 未知异常: {exc}")
        return JSONResponse(status_code=500, content={"error": "Internal Server Error"})
    finally:
        cleanup_account(request)
