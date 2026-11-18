package apis

import (
	"slices"
	"maps"
)

type ToolCall struct {
	Id       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name string `json:"name"`
	Args string `json:"arguments"`
}

type toolCallDelta struct {
	ToolCall
	Index int `json:"index"`
}

type toolCallBuffer map[int]ToolCall

// 将 toolCallDelta 块聚合
func (b toolCallBuffer) merge(deltas []toolCallDelta) {
	for _, delta := range deltas {
		curCall := b[delta.Index]

		if delta.Id != "" {
			curCall.Id = delta.Id
		}
		if delta.Type != "" {
			curCall.Type = delta.Type
		}
		if delta.Function.Name != "" {
			curCall.Function.Name = delta.Function.Name
		}

		curCall.Function.Args += delta.Function.Args
		b[delta.Index] = curCall
	}
}

func (b toolCallBuffer) getCalls() []ToolCall {
	calls := make([]ToolCall, 0, len(b))
	for _, index := range slices.Sorted(maps.Keys(b)) {
        calls = append(calls, b[index])
    }

	return calls
}

func CheckToolCalls(calls []ToolCall) error {
	// TODO
	return nil
}
