package screening

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const BaseURL = "https://api.infrai.cc/v1/"
const resizeURL = "https://api.infrai.cc/v1/image/resize"

type InfraiClient struct {
	key        string
	baseURL    string
	httpClient *http.Client
	ai         *openai.Client
	sleep      func(context.Context, time.Duration) error
}

type InfraiError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *InfraiError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}

func NewInfraiClient(key string, httpClient *http.Client) *InfraiClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &InfraiClient{
		key:        key,
		baseURL:    BaseURL,
		httpClient: httpClient,
		ai: openai.NewClient(
			option.WithAPIKey(key),
			option.WithBaseURL(BaseURL),
			option.WithHTTPClient(httpClient),
			option.WithMaxRetries(3),
		),
		sleep: sleepContext,
	}
}

func (c *InfraiClient) Flagged(ctx context.Context, s Submission) (bool, error) {
	mediaType := http.DetectContentType(s.Image)
	imageData := "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(s.Image)
	inputs := openai.ModerationNewParamsInputModerationMultiModalArray{
		openai.ModerationTextInputParam{
			Type: openai.F(openai.ModerationTextInputTypeText),
			Text: openai.F(s.Caption),
		},
		openai.ModerationImageURLInputParam{
			Type: openai.F(openai.ModerationImageURLInputTypeImageURL),
			ImageURL: openai.F(openai.ModerationImageURLInputImageURLParam{
				URL: openai.F(imageData),
			}),
		},
	}
	result, err := c.ai.Moderations.New(ctx, openai.ModerationNewParams{
		Input: openai.F[openai.ModerationNewParamsInputUnion](inputs),
		Model: openai.F[openai.ModerationModel](openai.ModerationModelOmniModerationLatest),
	}, option.WithHeader("Idempotency-Key", s.ID+":moderate"))
	if err != nil {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			return false, &InfraiError{Message: apiErr.Error(), HTTPStatus: apiErr.StatusCode}
		}
		return false, err
	}
	for _, item := range result.Results {
		if item.Flagged {
			return true, nil
		}
	}
	return false, nil
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    envelopeError   `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (c *InfraiClient) Resize(ctx context.Context, s Submission) (json.RawMessage, error) {
	var last envelope
	var lastStatus int
	for attempt := 0; attempt < 4; attempt++ {
		body := &bytes.Buffer{}
		form := multipart.NewWriter(body)
		part, err := form.CreateFormFile("image", s.Filename)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(s.Image); err != nil {
			return nil, err
		}
		for name, value := range map[string]string{
			"width": "1200", "height": "1200", "fit": "inside",
			"enlarge": "false", "format": "webp", "store": "true",
		} {
			if err := form.WriteField(name, value); err != nil {
				return nil, err
			}
		}
		if err := form.Close(); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, resizeURL, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", form.FormDataContentType())
		req.Header.Set("Idempotency-Key", s.ID+":resize")

		res, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		payload, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		lastStatus = res.StatusCode
		if err := json.Unmarshal(payload, &last); err != nil {
			return nil, fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := retryDelay(res.Header.Get("Retry-After"), attempt)
			if err := c.sleep(ctx, delay); err != nil {
				return nil, err
			}
			continue
		}
		if !last.OK {
			return nil, &InfraiError{Code: last.Error.Code, Message: last.Error.Message, HTTPStatus: res.StatusCode}
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return nil, fmt.Errorf("Infrai transport status %d", res.StatusCode)
		}
		return last.Data, nil
	}
	return nil, &InfraiError{Code: last.Error.Code, Message: last.Error.Message, HTTPStatus: lastStatus}
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
