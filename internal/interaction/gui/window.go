package gui

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"go-agent-framework/internal/agent"
	"go-agent-framework/internal/apis"
	"go-agent-framework/internal/config"
	"go-agent-framework/internal/storage"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type window struct {
	db              *sql.DB
	cfg             config.Config
	opts            apis.StreamOptions
	ctx             context.Context
	draft           string
	sessions        []storage.Session
	loops           map[int64]*agent.AgentLoop
	selectedSession storage.Session
	history         map[int64][]*chatMessage
	sessionErr      error
	win             *mygo.Window
	running         map[int64]bool
}

type chatMessage struct {
	apis.Message
	reasoningOpen bool
}

func (w *window) view(c *ui.Context) {
	ui.Row(c).Fill().Children(func() {
		w.sidebar(c)
		w.chat(c)
	})
}

func (w *window) sidebar(c *ui.Context) {
	ui.Column(c).Width(200).FillHeight().Children(func() {
		for _, session := range w.sessions {
			if ui.Button(c.Key(session.Id), session.Title).Clicked() {
				w.sessionErr = w.selectSession(session)
			}
		}
	})
}

func (w *window) chat(c *ui.Context) {
	title := w.selectedSession.Title
	chatMsgs := w.history[w.selectedSession.Id]

	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Text(c, title) // 标题栏

		if w.sessionErr != nil {
			ui.Text(c, fmt.Sprintf("切换会话失败: %v", w.sessionErr))
		}
		w.scrollChat(c, chatMsgs) // 对话记录栏

		ui.TextArea(c, &w.draft) // 输入框
		w.submitButton(c)        // 发送按钮
	})
}

func (w *window) scrollChat(c *ui.Context, chatMsgs []*chatMessage) {
	ui.Scroll(c.Key(w.selectedSession.Id)).Grow(1).Children(func() {
		for _, chatMsg := range chatMsgs {
			ui.Column(c.Key(chatMsg)).Children(func() {
				switch chatMsg.Role {
				case "user":
					ui.Text(c, fmt.Sprintf("[用户] %s\n", chatMsg.Content))
				case "assistant":
					if chatMsg.ReasoningContent != "" { //思考内容
						ui.Collapsible(
							c,
							"思考",
							&chatMsg.reasoningOpen,
							func() {
								ui.Text(c, fmt.Sprintf("[思考内容] %s\n", chatMsg.ReasoningContent))
							},
						)
					}
					if chatMsg.Content != "" { // 正文内容
						ui.Text(c, fmt.Sprintf("[回复] %s\n", chatMsg.Content))
					}
					for _, call := range chatMsg.ToolCalls { // 工具调用
						ui.Text(c, fmt.Sprintf("[工具调用] %s %s\n", call.Function.Name, call.Function.Args))
					}
				case "tool": // 工具结果
					ui.Text(c, fmt.Sprintf("[工具结果] %s\n", chatMsg.Content))
				default:
					ui.Text(c, fmt.Sprintf("%s\n", chatMsg.Content))
				}
			})
		}
	})
}

func (w *window) submitButton(c *ui.Context) {
	if ui.Button(c, "提交").Clicked() {
		if strings.TrimSpace(w.draft) != "" {
			submitId := w.selectedSession.Id
			submitDraft := w.draft

			if submitId == 0 || strings.TrimSpace(submitDraft) == "" {
				return
			}

			w.submit(submitId, submitDraft)
		}
	}
}

// 加载指定 session
func (w *window) selectSession(session storage.Session) error {
	if _, loaded := w.history[session.Id]; !loaded { // 加载指定 sessionId 的历史
		msgs, err := agent.LoadHistory(w.ctx, w.db, session.Id)
		if err != nil {
			return err
		}

		chatMsgs := make([]*chatMessage, 0, len(msgs)) // 填充history
		for _, msg := range msgs {
			chatMsgs = append(chatMsgs, &chatMessage{Message: msg})
		}

		w.history[session.Id] = chatMsgs
	}

	w.selectedSession = session
	return nil
}

func (w *window) submit(submitId int64, submitDraft string) {
	if w.running[submitId] {
		return
	}

	submitLoop := w.loops[submitId]
	if submitLoop == nil {
		submitLoop = agent.NewAgentLoop(w.cfg, w.db, w.opts)
		if err := submitLoop.BindSession(w.ctx, submitId); err != nil {
			w.sessionErr = err
			return
		}

		w.loops[submitId] = submitLoop
	}

	inputFunc := func(ctx context.Context) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		return submitDraft, nil
	}
	var reply *chatMessage
	outputFunc := func(ctx context.Context, chunk string, role agent.OutputRole) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		w.win.Update(func() { // 更新窗口
			if role == agent.IsToolContent {
				toolMsg := &chatMessage{
					Message: apis.Message{Role: "tool", Content: chunk},
				}
				w.history[submitId] = append(w.history[submitId], toolMsg)
				reply = nil
				return
			}

			if reply == nil {
				reply = &chatMessage{
					Message: apis.Message{Role: "assistant"},
				}
				w.history[submitId] = append(w.history[submitId], reply)
			}

			switch role {
			case agent.IsReasoningContent:
				reply.ReasoningContent += chunk
			case agent.IsContent:
				reply.Content += chunk
			}
		})

		return nil
	}
	confirmFunc := func(ctx context.Context, dir, command string) (bool, error) {
		return false, ctx.Err()
	}

	w.running[submitId] = true
	w.draft = ""

	userMsg := &chatMessage{
		Message: apis.Message{Role: "user", Content: submitDraft},
	}
	w.history[submitId] = append(w.history[submitId], userMsg) // 追加窗口对话历史

	go func() { // 后台提交
		err := submitLoop.RunTurn(w.ctx, inputFunc, outputFunc, confirmFunc)

		w.win.Update(func() {
			delete(w.running, submitId)

			if err != nil {
				fmt.Printf("会话 %d 提交失败: %v", submitId, err)
			}
		})
	}()
}

func Run(ctx context.Context, db *sql.DB, cfg config.Config, opts apis.StreamOptions) error {
	sessions, err := storage.ListSessions(ctx, db)
	if err != nil {
		return err
	}

	w := &window{
		db:       db,
		ctx:      ctx,
		cfg:      cfg,
		opts:     opts,
		sessions: sessions,
		history:  make(map[int64][]*chatMessage),
		loops:    make(map[int64]*agent.AgentLoop),
		running:  make(map[int64]bool),
	}

	if len(sessions) > 0 {
		if err := w.selectSession(sessions[0]); err != nil {
			return err
		}
	}

	mygo.App.WhenReady(func() {
		w.win = mygo.NewWindow(mygo.WindowOptions{
			Title:   "SAISON",
			Width:   960,
			Height:  640,
			Content: ui.View(w.view),
		})
	})

	return mygo.App.Run()
}
