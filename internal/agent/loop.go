package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"go-agent-framework/internal/apis"
	"go-agent-framework/internal/config"
	"go-agent-framework/internal/storage"
	"go-agent-framework/internal/tools"
)

type AgentLoop struct {
	SessionId int64 // 绑定会话Id
	cfg       config.Config
	msgs      []apis.Message // 本会话的对话历史
	db        *sql.DB
	opts      apis.StreamOptions
}

type InputFunc func(context.Context) (string, error)
type OutputFunc func(context.Context, string, bool) error // bool = isReasoning
type ConfirmFunc func(context.Context, string, string) (bool, error)

type bashArgs struct {
	Command string `json:"command"`
}

func NewAgentLoop(cfg config.Config, db *sql.DB, opts apis.StreamOptions) *AgentLoop {
	return &AgentLoop{
		cfg:  cfg,
		db:   db,
		opts: opts,
	}
}

func (l *AgentLoop) NewSession(ctx context.Context, title string) error {
	sessionId, err := storage.CreateSession(ctx, l.db, title)
	if err != nil {
		return err
	}

	l.SessionId = sessionId
	l.msgs = make([]apis.Message, 0)
	return nil
}

// 将 Loop 绑定到会话
func (l *AgentLoop) BindSession(ctx context.Context, sessionId int64) error {
	// 查询sessionId

	// 载入msgs，cfg，opts
	return nil
}

// 推进对话轮次
func (l *AgentLoop) RunTurn(ctx context.Context, inputFunc InputFunc, outputFunc OutputFunc, confirmFunc ConfirmFunc) error {
	input, err := l.readInput(ctx, inputFunc) // 读取输入
	if err != nil {
		return err
	}
	if err := l.appendMessage(ctx, input); err != nil {
		return err
	}

	needReply := true
	for needReply { // 读取输出循环
		output, finishReason, err := l.readReply(ctx, outputFunc)
		if err != nil {
			return err
		}

		if finishReason != "tool_calls" {
			needReply = false // 非工具调用，进入下一轮

			if output.Role == "" {
				continue
			}
		} else {
			if err := apis.CheckToolCalls(output.ToolCalls); err != nil {
				return err
			}
		}

		if err := l.appendMessage(ctx, output); err != nil { // 对话历史追加并储存
			return err
		}

		for _, call := range output.ToolCalls { // 遍历工具调用结果，追加并储存
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			toolOutput, err := l.runToolCall(ctx, call, dir, confirmFunc)
			if err != nil {
				return err
			}

			msg := apis.Message{
				Role:       "tool",
				Content:    toolOutput,
				ToolCallId: call.Id,
			}

			if err := l.appendMessage(ctx, msg); err != nil {
				return err
			}
		}
	}

	return nil
}

// 执行工具调用
func (l *AgentLoop) runToolCall(ctx context.Context, call apis.ToolCall, dir string, confirm ConfirmFunc) (string, error) {
	args := strings.TrimSpace(call.Function.Args)
	if !strings.HasPrefix(args, "{") || !json.Valid([]byte(args)) { // 合法性检查
		return fmt.Sprintf("工具调用 %s 的参数必须是 JSON 对象", call.Id), nil
	}

	switch call.Function.Name {
	case "bash":
		bashArgs := bashArgs{}
		decoder := json.NewDecoder(strings.NewReader(args))
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&bashArgs); err != nil { // Command 命令串
			return fmt.Sprintf("bash 参数解析错误: %v", err), nil
		}

		if strings.TrimSpace(bashArgs.Command) == "" {
			return "bash 命令串为空", nil
		}

		isConfirmed, err := confirm(ctx, dir, bashArgs.Command)
		if err != nil {
			return "", err
		}
		if !isConfirmed {
			return "用户拒绝执行", nil
		}

		toolOutput, err := tools.ExecBash(ctx, dir, bashArgs.Command)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if err != nil {
			return fmt.Sprintf("bash 执行失败: %v\n%s", err, toolOutput), nil
		}

		return toolOutput, nil
	default:
		return "尚未实现", nil
	}
}

// 读取输入
func (l *AgentLoop) readInput(ctx context.Context, inputFunc InputFunc) (apis.Message, error) {
	input, err := inputFunc(ctx)
	if err != nil {
		return apis.Message{}, err
	}

	msg := apis.Message{
		Role:    "user",
		Content: input,
	}
	return msg, nil
}

// 读取模型回复
func (l *AgentLoop) readReply(ctx context.Context, outputFunc OutputFunc) (apis.Message, string, error) {
	var output, reasoningOutput strings.Builder // 输出内容缓冲区
	var calls []apis.ToolCall                   // 记录工具调用
	var finishReason string
	var reasoning bool

	// 流式解析SSE
	for event, err := range apis.StreamChat(ctx, l.msgs, l.opts, l.cfg) { // 解析sse流
		if err != nil {
			return apis.Message{}, "", fmt.Errorf("流式对话失败: %w", err)
		}
		switch event.Kind {
		case apis.EventReasoning: // 思考内容
			reasoning = true
			reasoningOutput.WriteString(event.Text)
			if err := outputFunc(ctx, event.Text, true); err != nil {
				return apis.Message{}, "", err
			}
		case apis.EventContent: // 输出内容
			splitMark := ""
			if reasoning == true {
				reasoning = false
				splitMark = "\n"
			}
			output.WriteString(event.Text)
			if err := outputFunc(ctx, splitMark+event.Text, false); err != nil {
				return apis.Message{}, "", err
			}
		case apis.EventFinish: // 结束原因
			finishReason = event.FinishReason
			if event.FinishReason == "tool_calls" {
				calls = event.ToolCalls
			}
		case apis.EventDone: // 轮次结束标识
			msg := apis.Message{}
			if output.Len() > 0 || reasoningOutput.Len() > 0 || len(calls) > 0 {
				msg = apis.Message{
					Role:             "assistant",
					Content:          output.String(),
					ReasoningContent: reasoningOutput.String(),
					ToolCalls:        calls,
				}
			}
			return msg, finishReason, nil
		}
	}

	return apis.Message{}, "", io.ErrUnexpectedEOF
}

// 将 msg 追加至对话历史，并持久化储存
func (l *AgentLoop) appendMessage(ctx context.Context, msg apis.Message) error {
	callsString := "[]"
	if len(msg.ToolCalls) > 0 {
		data, err := json.Marshal(msg.ToolCalls)
		if err != nil {
			return fmt.Errorf("追加对话历史，工具调用转换失败: %w", err)
		}
		callsString = string(data)
	}

	msgStorage := storage.Message{
		SessionId:        l.SessionId,
		Role:             msg.Role,
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
		ToolCalls:        callsString,
		ToolCallId:       msg.ToolCallId,
	}
	if err := storage.CreateMessage(ctx, l.db, msgStorage); err != nil { // 储存模型输出
		return fmt.Errorf("追加对话历史，储存失败: %w", err)
	}

	l.msgs = append(l.msgs, msg)

	return nil
}
