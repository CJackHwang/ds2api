package openai

import "regexp"

var visibleToolXMLBlockPatterns = []*regexp.Regexp{
regexp.MustCompile(`(?is)<tool_calls\b[^>]*>[\s\S]*?</tool_calls>`),
regexp.MustCompile(`(?is)<tool_call\b[^>]*>[\s\S]*?</tool_call>`),
regexp.MustCompile(`(?is)<tool_calls\b[^>]*>[\s\S]*$`),
regexp.MustCompile(`(?is)<tool_call\b[^>]*>[\s\S]*$`),
}

func stripVisibleToolCallMarkup(text string) string {
out := text
for _, pattern := range visibleToolXMLBlockPatterns {
out = pattern.ReplaceAllString(out, "")
}
out = leakedToolCallsWrapperFragmentPattern.ReplaceAllString(out, "")
out = leakedToolCallClosingFragmentPattern.ReplaceAllString(out, "")
return out
}
