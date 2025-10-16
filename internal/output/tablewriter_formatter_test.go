package output

import (
	"testing"
)

func TestShortenModelName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
		desc     string
	}{
		// 新的 4.1 和 4.5 格式
		{"claude-opus-4-1-20250805", "Opus-4.1", "Opus 4.1 模型"},
		{"claude-sonnet-4-5-20250929", "Sonnet-4.5", "Sonnet 4.5 模型"},
		{"claude-haiku-4-5-20251001", "Haiku-4.5", "Haiku 4.5 模型"},

		// 原有的標準格式
		{"claude-opus-4-20250514", "Opus-4", "Opus 4 標準格式"},
		{"claude-sonnet-4-20250514", "Sonnet-4", "Sonnet 4 標準格式"},
		{"claude-haiku-3-20240307", "Haiku-3", "Haiku 3 模型"},

		// 非 Claude 模型
		{"gpt-4o", "gpt-4o", "GPT-4o 模型"},
		{"gpt-4o-mini", "gpt-4o-mini", "GPT-4o-mini 模型"},
		{"gpt-4", "gpt-4", "GPT-4 模型"},
		{"gpt-3.5-turbo", "gpt-3.5", "GPT-3.5 模型"},

		// 未知模型格式
		{"some-unknown-model", "some-unknown", "未知模型（截斷）"},
		{"very-long-model-name-that-exceeds-limit", "very-long-mo", "超長模型名稱（截斷到12字元）"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result := ShortenModelName(tc.input)
			if result != tc.expected {
				t.Errorf("輸入 %s: 預期 %s，實際得到 %s", tc.input, tc.expected, result)
			}
		})
	}
}