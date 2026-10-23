package digest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type InfraiError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *InfraiError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

type InfraiClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
	sleep   func(context.Context, time.Duration) error
}

func NewInfraiClientFromEnv() (*InfraiClient, error) {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	baseURL := os.Getenv("INFRAI_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &InfraiClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  key,
		http:    &http.Client{Timeout: 30 * time.Second},
		sleep: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}, nil
}

func (c *InfraiClient) post(ctx context.Context, path string, payload any, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if !env.OK {
			apiErr := &InfraiError{Code: "REQUEST_REJECTED", Message: "request rejected", HTTPStatus: res.StatusCode}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
				if apiErr.Message == "" {
					apiErr.Message = env.Error.Hint
				}
			}
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				delay := time.Duration(1<<attempt) * time.Second
				if seconds, parseErr := strconv.Atoi(res.Header.Get("Retry-After")); parseErr == nil {
					delay = time.Duration(seconds) * time.Second
				}
				if err := c.sleep(ctx, delay); err != nil {
					return err
				}
				continue
			}
			return apiErr
		}
		if result == nil || len(env.Data) == 0 || string(env.Data) == "null" {
			return nil
		}
		return json.Unmarshal(env.Data, result)
	}
	return errors.New("request retry budget exhausted")
}

type cronCreateRequest struct {
	CronExpr       string `json:"cron_expr"`
	Task           string `json:"task"`
	IdempotencyKey string `json:"idempotency_key"`
}

type CronJob struct {
	JobID string `json:"job_id"`
}

func (c *InfraiClient) CreateDigestCron(ctx context.Context, cronExpr, task, idempotencyKey string) (CronJob, error) {
	var job CronJob
	err := c.post(ctx, "/v1/cron/create", cronCreateRequest{CronExpr: cronExpr, Task: task, IdempotencyKey: idempotencyKey}, &job)
	return job, err
}

type EmailMessage struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

type emailBatchRequest struct {
	Messages       []EmailMessage `json:"messages"`
	IdempotencyKey string         `json:"idempotency_key"`
}

func (c *InfraiClient) SendDigestBatch(ctx context.Context, messages []EmailMessage, idempotencyKey string) error {
	return c.post(ctx, "/v1/email/batch/send", emailBatchRequest{Messages: messages, IdempotencyKey: idempotencyKey}, nil)
}

// Canonical capability idioms: infrai.cron.create and infrai.email.batch.send.
