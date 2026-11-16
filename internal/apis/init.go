package apis

import (
	"net/http"
	"time"
)

func initStreamReqHeader(req *http.Request, apiKey string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+apiKey)
}

func initHttpClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			ResponseHeaderTimeout: 60 * time.Second,
		},
	}
}
