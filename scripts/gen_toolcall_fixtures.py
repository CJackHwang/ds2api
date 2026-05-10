#!/usr/bin/env python3
"""M0-A: Extract tool-call fixtures from tests/chat_history.json.d/"""
import json
import os
import re

BASE = os.path.join(os.path.dirname(__file__), "..", "tests", "chat_history.json.d")
TP_DIR = os.path.join(os.path.dirname(__file__), "..", "tests", "compat", "fixtures", "toolcalls", "true_positive")
FP_DIR = os.path.join(os.path.dirname(__file__), "..", "tests", "compat", "fixtures", "toolcalls", "false_positive")


def load_content(fname: str) -> str:
    with open(os.path.join(BASE, fname), encoding="utf-8") as f:
        data = json.load(f)
    return data["item"].get("content", "")


def extract_tool_names(content: str) -> list[str]:
    # Match: invoke name="ToolName" in any DSML variant
    hits = re.findall(r'invoke\s+name\s*=\s*["\u201c]([^"\u201d\]>]+)', content)
    cleaned = []
    for h in hits:
        name = h.split('"')[0].split("'")[0].split(">")[0].strip()
        if name and name not in cleaned:
            cleaned.append(name)
    return cleaned


def write_json(path: str, obj: object) -> None:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(obj, f, ensure_ascii=False, indent=2)
    print(f"  wrote {os.path.relpath(path)}")


# ---------------------------------------------------------------------------
# TRUE POSITIVE specs: (output_filename, source_chat_history_file)
# ---------------------------------------------------------------------------
TP_SPECS = [
    ("dsml_single_cdata_read.json",      "chat_02f244dfc5444071988a76f1615e57dd.json"),
    ("dsml_single_edit_multiparams.json", "chat_08e2f7fe05ea497cbd061a0c4561d2ba.json"),
    ("dsml_single_read_offset.json",     "chat_4194b5a370a744d59eb11e1cde4f2442.json"),
    ("dsml_multi_read_pair.json",        "chat_41f9aa2bbabb47dbb049a800e44a27a4.json"),
    ("dsml_multi_read_trio.json",        "chat_94c6f11d8ce2435e94c9885341b936ff.json"),
    ("dsml_drift_no_open_pipe.json",     "chat_7a52fd84243d4aabb1c2839af8d73877.json"),
    ("dsml_drift_bash_command.json",     "chat_9d19122eb404427d866c10d51493c527.json"),
]

# ---------------------------------------------------------------------------
# FALSE POSITIVE specs: (output_filename, source_file, forced_tool_names)
# ---------------------------------------------------------------------------
FP_SPECS = [
    ("code_block_dpr_analysis.json",    "chat_117b05c673f34c6ca8f314d5f2c3690f.json", ["Read", "Edit", "Bash"]),
    ("dsml_like_wrapper_only.json",     "chat_9c447b70813a4d978135e5edb06986a2.json", ["Read", "Edit", "Bash"]),
    ("dsml_no_separator_variant.json",  "chat_5f9480bd9c6741b6a4ec8456c1f07c89.json", ["Read", "Edit", "Bash"]),
]

FENCED_TEXT = (
    "```xml\n"
    '<tool_calls><invoke name="Read">'
    '<parameter name="path">README.md</parameter>'
    "</invoke></tool_calls>\n"
    "```\n\n"
    "这里只是展示格式，不是真实调用。"
)

PLAIN_HTML = (
    '<div class="tool_calls">\n'
    '  <span class="invoke">just HTML markup</span>\n'
    "</div>"
)


def main() -> None:
    print("=== true_positive ===")
    for out_name, src in TP_SPECS:
        content = load_content(src)
        names = extract_tool_names(content)
        write_json(os.path.join(TP_DIR, out_name), {"text": content, "tool_names": names})

    print("=== false_positive ===")
    for out_name, src, forced in FP_SPECS:
        content = load_content(src)
        write_json(os.path.join(FP_DIR, out_name), {"text": content, "tool_names": forced})

    write_json(
        os.path.join(FP_DIR, "fenced_code_block_tool_call.json"),
        {"text": FENCED_TEXT, "tool_names": ["Read"]},
    )
    write_json(
        os.path.join(FP_DIR, "plain_html_classname.json"),
        {"text": PLAIN_HTML, "tool_names": ["Read", "Edit"]},
    )

    print("Done.")


if __name__ == "__main__":
    main()
