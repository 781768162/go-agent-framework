package apis

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const apiKey = "should-read-from-config"
var client *http.Client = initHttpClient() 

type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func handleStream(ctx context.Context, client *http.Client, endpoint string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}

	initStreamReqHeader(req)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.New("请求失败")
	}

	parseStreamBody(resp.Body)
	
}

func initStreamReqHeader(req *http.Request) {
	if req == nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+apiKey)
}

func initHttpClient() *http.Client {
	return &http.Client{
		Timeout: time.Duration(1000 * time.Second),
	}
}

func parseStreamBody(body io.Reader) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), 1<<20)

	for scanner.Scan() {
		line := scanner.Text() // data:原始行
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue // 这会跳过空行和心跳
		}
		data = strings.TrimPrefix(data, " ")
		data = strings.TrimSuffix(data, " ")

		var chunk chatChunk
		if err := json.Unmarshal([]byte(), &chunk); err != nil {
			return fmt.Errorf("解析SSE data失败: %w", err)
		}
		if len(chunk.Choices) > 0 {
		
		}
	}
}