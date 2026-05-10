#!/usr/bin/env python3
"""M0-B: Extract context-engine fixtures from tests/chat_history.json.d/"""
import json
import os

BASE = os.path.join(os.path.dirname(__file__), "..", "tests", "chat_history.json.d")
OUT = os.path.join(os.path.dirname(__file__), "..", "tests", "compat", "fixtures", "context")


def load_item(fname: str) -> dict:
    with open(os.path.join(BASE, fname), encoding="utf-8") as f:
        return json.load(f)["item"]


def write_json(path: str, obj: object) -> None:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(obj, f, ensure_ascii=False, indent=2)
    print(f"  wrote {os.path.relpath(path)}")


def main() -> None:
    # ------------------------------------------------------------------ #
    # Fixture 1: plain multi-turn (short messages, no tool call)
    # Use chat_117b05 which has messages + history_text (pure text, code blocks)
    # ------------------------------------------------------------------ #
    item1 = load_item("chat_117b05c673f34c6ca8f314d5f2c3690f.json")
    write_json(
        os.path.join(OUT, "plain_multiturn.json"),
        {
            "description": "Multi-turn conversation with code-block analysis; no tool calls. "
            "Tests that the compiler preserves all segments without orphan detection.",
            "messages": item1.get("messages", []),
            "history_text": item1.get("history_text", "")[:8000],  # first 8k chars for readability
            "full_history_len": len(item1.get("history_text", "")),
            "file_refs": [],
        },
    )

    # ------------------------------------------------------------------ #
    # Fixture 2: tool loop sequence (assistant tool_call -> tool result)
    # Constructed from chat_02f244 (DSML Read call)
    # ------------------------------------------------------------------ #
    item2 = load_item("chat_02f244dfc5444071988a76f1615e57dd.json")
    assistant_content = item2.get("content", "")
    tool_loop_messages = [
        {
            "role": "user",
            "content": item2.get("user_input", "请继续分析代码"),
        },
        {
            "role": "assistant",
            "content": assistant_content,
            "tool_calls": [
                {
                    "id": "call_01",
                    "type": "function",
                    "function": {
                        "name": "Read",
                        "arguments": json.dumps(
                            {
                                "file_path": "/Users/activer/developer/game-01/game.js",
                                "limit": 60,
                                "offset": 990,
                            }
                        ),
                    },
                }
            ],
        },
        {
            "role": "tool",
            "tool_call_id": "call_01",
            "name": "Read",
            "content": "// game.js lines 990-1050\nfunction _drawNext() {\n  // ...placeholder content...\n}\n",
        },
    ]
    write_json(
        os.path.join(OUT, "tool_loop_read.json"),
        {
            "description": "Assistant makes a Read tool call; paired tool result follows. "
            "Tests tool_call/tool_result pair invariant validation.",
            "messages": tool_loop_messages,
            "history_text": "",
            "file_refs": [],
        },
    )

    # ------------------------------------------------------------------ #
    # Fixture 3: orphan tool_call (no tool result — truncation scenario)
    # ------------------------------------------------------------------ #
    orphan_messages = [
        {"role": "user", "content": "请读取 game.js 第 990 行"},
        {
            "role": "assistant",
            "content": "好的，我来读取文件。",
            "tool_calls": [
                {
                    "id": "call_02",
                    "type": "function",
                    "function": {
                        "name": "Read",
                        "arguments": json.dumps(
                            {
                                "file_path": "/Users/activer/developer/game-01/game.js",
                                "limit": 60,
                                "offset": 990,
                            }
                        ),
                    },
                }
            ],
        },
        # Deliberately omitting the tool result — this is the orphan scenario
    ]
    write_json(
        os.path.join(OUT, "orphan_tool_call.json"),
        {
            "description": "Assistant has tool_calls but the matching tool result is missing "
            "(history truncated mid-loop). Context Engine must detect this invariant violation.",
            "messages": orphan_messages,
            "history_text": "",
            "file_refs": [],
            "expected_issues": ["orphan_tool_call"],
        },
    )

    # ------------------------------------------------------------------ #
    # Fixture 4: long history — token budget pressure
    # Use chat_ac77 (525k history_text) — truncated to 16k for fixture
    # ------------------------------------------------------------------ #
    item4 = load_item("chat_ac77992c2f9f4103838e8c4d949a4254.json")
    full_len = len(item4.get("history_text", ""))
    # Use first 16k chars; Context Engine token budget logic should clip/trim
    write_json(
        os.path.join(OUT, "long_history_token_budget.json"),
        {
            "description": "Very long history_text (full: %d chars, fixture: 16000 chars). "
            "Tests that the Context Engine respects token budget and trims appropriately "
            "while preserving the most-recent segments." % full_len,
            "messages": item4.get("messages", []),
            "history_text": item4.get("history_text", "")[:16000],
            "full_history_len": full_len,
            "file_refs": [],
            "token_budget_hint": 4096,
        },
    )

    print("Done. Created 4 context fixtures.")


if __name__ == "__main__":
    main()
