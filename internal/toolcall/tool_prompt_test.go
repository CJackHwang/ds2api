package toolcall

import (
	"strings"
	"testing"
)

func TestBuildToolCallInstructions_ExecCommandUsesCmdExample(t *testing.T) {
	out := BuildToolCallInstructions([]string{"exec_command"})
	if !strings.Contains(out, `<|DSML|invoke name="exec_command">`) {
		t.Fatalf("expected exec_command in examples, got: %s", out)
	}
	if !strings.Contains(out, `<|DSML|parameter name="cmd"><![CDATA[pwd]]></|DSML|parameter>`) {
		t.Fatalf("expected cmd parameter example for exec_command, got: %s", out)
	}
}

func TestBuildToolCallInstructions_ExecuteCommandUsesCommandExample(t *testing.T) {
	out := BuildToolCallInstructions([]string{"execute_command"})
	if !strings.Contains(out, `<|DSML|invoke name="execute_command">`) {
		t.Fatalf("expected execute_command in examples, got: %s", out)
	}
	if !strings.Contains(out, `<|DSML|parameter name="command"><![CDATA[pwd]]></|DSML|parameter>`) {
		t.Fatalf("expected command parameter example for execute_command, got: %s", out)
	}
}

func TestBuildToolCallInstructions_BashUsesCommandAndDescriptionExamples(t *testing.T) {
	out := BuildToolCallInstructions([]string{"Bash"})
	blocks := findInvokeBlocks(out, "Bash")
	if len(blocks) == 0 {
		t.Fatalf("expected Bash examples, got: %s", out)
	}

	sawDescription := false
	for _, block := range blocks {
		if !strings.Contains(block, `<|DSML|parameter name="command">`) {
			t.Fatalf("expected every Bash example to use command parameter, got: %s", block)
		}
		if strings.Contains(block, `<|DSML|parameter name="path">`) || strings.Contains(block, `<|DSML|parameter name="content">`) {
			t.Fatalf("expected Bash examples not to use file write parameters, got: %s", block)
		}
		if strings.Contains(block, `<|DSML|parameter name="description">`) {
			sawDescription = true
		}
	}
	if !sawDescription {
		t.Fatalf("expected Bash long-script example to include description, got: %s", out)
	}
	if strings.Contains(out, `<|DSML|invoke name="Read">`) {
		t.Fatalf("expected examples to avoid unavailable hard-coded Read tool, got: %s", out)
	}
}

func TestBuildToolCallInstructions_ExecuteCommandLongScriptUsesCommand(t *testing.T) {
	out := BuildToolCallInstructions([]string{"execute_command"})
	blocks := findInvokeBlocks(out, "execute_command")
	if len(blocks) == 0 {
		t.Fatalf("expected execute_command examples, got: %s", out)
	}

	for _, block := range blocks {
		if !strings.Contains(block, `<|DSML|parameter name="command">`) {
			t.Fatalf("expected execute_command examples to use command parameter, got: %s", block)
		}
		if strings.Contains(block, `<|DSML|parameter name="path">`) || strings.Contains(block, `<|DSML|parameter name="content">`) {
			t.Fatalf("expected execute_command examples not to use file write parameters, got: %s", block)
		}
	}
	if !strings.Contains(out, `test_escape.sh`) {
		t.Fatalf("expected execute_command long-script example, got: %s", out)
	}
}

func TestBuildToolCallInstructions_ExecCommandLongScriptUsesCmd(t *testing.T) {
	out := BuildToolCallInstructions([]string{"exec_command"})
	blocks := findInvokeBlocks(out, "exec_command")
	if len(blocks) == 0 {
		t.Fatalf("expected exec_command examples, got: %s", out)
	}

	for _, block := range blocks {
		if !strings.Contains(block, `<|DSML|parameter name="cmd">`) {
			t.Fatalf("expected exec_command examples to use cmd parameter, got: %s", block)
		}
		if strings.Contains(block, `<|DSML|parameter name="command">`) || strings.Contains(block, `<|DSML|parameter name="path">`) || strings.Contains(block, `<|DSML|parameter name="content">`) {
			t.Fatalf("expected exec_command examples not to use command or file write parameters, got: %s", block)
		}
	}
	if !strings.Contains(out, `test_escape.sh`) {
		t.Fatalf("expected exec_command long-script example, got: %s", out)
	}
}

func TestBuildToolCallInstructions_WriteUsesFilePathAndContent(t *testing.T) {
	out := BuildToolCallInstructions([]string{"Write"})
	blocks := findInvokeBlocks(out, "Write")
	if len(blocks) == 0 {
		t.Fatalf("expected Write examples, got: %s", out)
	}

	for _, block := range blocks {
		if !strings.Contains(block, `<|DSML|parameter name="file_path">`) || !strings.Contains(block, `<|DSML|parameter name="content">`) {
			t.Fatalf("expected Write examples to use file_path and content, got: %s", block)
		}
		if strings.Contains(block, `<|DSML|parameter name="path">`) {
			t.Fatalf("expected Write examples not to use path, got: %s", block)
		}
	}
}

func TestBuildToolCallInstructions_AnchorsMissingOpeningWrapperFailureMode(t *testing.T) {
	out := BuildToolCallInstructions([]string{"read_file"})
	if !strings.Contains(out, "Never omit the opening <|DSML|tool_calls> tag") {
		t.Fatalf("expected explicit missing-opening-tag warning, got: %s", out)
	}
	if !strings.Contains(out, "Wrong 3 — missing opening wrapper") {
		t.Fatalf("expected missing-opening-wrapper negative example, got: %s", out)
	}
}

func TestBuildToolCallInstructions_RejectsEmptyParametersInPrompt(t *testing.T) {
	out := BuildToolCallInstructions([]string{"Bash"})
	for _, want := range []string{
		"Do not emit placeholder, blank, or whitespace-only parameters.",
		"If a required parameter value is unknown, ask the user or answer normally instead of outputting an empty tool call.",
		"Never call them with an empty command.",
		"Wrong 4 — empty parameters",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected empty-parameter instruction %q, got: %s", want, out)
		}
	}
}

func TestBuildToolCallInstructions_UsesPositiveTagPunctuationAlphabet(t *testing.T) {
	out := BuildToolCallInstructions([]string{"Bash"})
	want := `Tag punctuation alphabet: ASCII < > / = " plus the halfwidth pipe |.`
	if !strings.Contains(out, want) {
		t.Fatalf("expected positive tag punctuation alphabet %q, got: %s", want, out)
	}
	for _, bad := range []string{"lookalike", "substitute", "！", "〈", "〉", "“", "”", "、"} {
		if strings.Contains(out, bad) {
			t.Fatalf("tool prompt should not include negative punctuation examples %q, got: %s", bad, out)
		}
	}
}

func findInvokeBlocks(text, name string) []string {
	open := `<|DSML|invoke name="` + name + `">`
	remaining := text
	blocks := []string{}
	for {
		start := strings.Index(remaining, open)
		if start < 0 {
			return blocks
		}
		remaining = remaining[start:]
		end := strings.Index(remaining, `</|DSML|invoke>`)
		if end < 0 {
			return blocks
		}
		end += len(`</|DSML|invoke>`)
		blocks = append(blocks, remaining[:end])
		remaining = remaining[end:]
	}
}
