package toolcall

import (
	"reflect"
	"testing"
)

func TestRegression_RobustXMLAndCDATA(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []ParsedToolCall
	}{
		{
			name:     "Standard JSON parameters (Regression)",
			text:     `<tool_call><tool_name>foo</tool_name><parameters>{"a": 1}</parameters></tool_call>`,
			expected: []ParsedToolCall{{Name: "foo", Input: map[string]any{"a": float64(1)}}},
		},
		{
			name:     "XML tags parameters (Regression)",
			text:     `<tool_call><tool_name>foo</tool_name><parameters><arg1>hello</arg1></parameters></tool_call>`,
			expected: []ParsedToolCall{{Name: "foo", Input: map[string]any{"arg1": "hello"}}},
		},
		{
			name: "CDATA parameters (New Feature)",
			text: `<tool_call><tool_name>write_file</tool_name><parameters><content><![CDATA[line 1
line 2 with <tags> and & symbols]]></content></parameters></tool_call>`,
			expected: []ParsedToolCall{{
				Name:  "write_file",
				Input: map[string]any{"content": "line 1\nline 2 with <tags> and & symbols"},
			}},
		},
		{
			name: "Nested XML with repeated parameters (New Feature)",
			text: `<tool_call><tool_name>write_file</tool_name><parameters><path>script.sh</path><content><![CDATA[#!/bin/bash
echo "hello"
]]></content><item>first</item><item>second</item></parameters></tool_call>`,
			expected: []ParsedToolCall{{
				Name: "write_file",
				Input: map[string]any{
					"path":    "script.sh",
					"content": "#!/bin/bash\necho \"hello\"\n",
					"item":    []any{"first", "second"},
				},
			}},
		},
		{
			name: "Dirty XML with unescaped symbols (Robustness Improvement)",
			text: `<tool_call><tool_name>bash</tool_name><parameters><command>echo "hello" > out.txt && cat out.txt</command></parameters></tool_call>`,
			expected: []ParsedToolCall{{
				Name:  "bash",
				Input: map[string]any{"command": "echo \"hello\" > out.txt && cat out.txt"},
			}},
		},
		{
			name: "Mixed JSON inside CDATA (New Hybrid Case)",
			text: `<tool_call><tool_name>foo</tool_name><parameters><![CDATA[{"json_param": "works"}]]></parameters></tool_call>`,
			expected: []ParsedToolCall{{
				Name:  "foo",
				Input: map[string]any{"json_param": "works"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseToolCalls(tt.text, []string{"foo", "write_file", "bash"})
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d calls, got %d", len(tt.expected), len(got))
			}
			for i := range got {
				if got[i].Name != tt.expected[i].Name {
					t.Errorf("expected name %q, got %q", tt.expected[i].Name, got[i].Name)
				}
				if !reflect.DeepEqual(got[i].Input, tt.expected[i].Input) {
					t.Errorf("expected input %#v, got %#v", tt.expected[i].Input, got[i].Input)
				}
			}
		})
	}
}
