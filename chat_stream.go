package openai

import (
	"context"
	"net/http"
)

type ChatCompletionStreamChoiceDelta struct {
	Content      string        `json:"content,omitempty"`
	Role         string        `json:"role,omitempty"`
	FunctionCall *FunctionCall `json:"function_call,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	Refusal      string        `json:"refusal,omitempty"`

	// This property is used for the "reasoning" feature supported by deepseek-reasoner
	// which is not in the official documentation.
	// the doc from deepseek:
	// - https://api-docs.deepseek.com/api/create-chat-completion#responses
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type ChatCompletionStreamChoiceLogprobs struct {
	Content []ChatCompletionTokenLogprob `json:"content,omitempty"`
	Refusal []ChatCompletionTokenLogprob `json:"refusal,omitempty"`
}

type ChatCompletionTokenLogprob struct {
	Token       string                                 `json:"token"`
	Bytes       []int64                                `json:"bytes,omitempty"`
	Logprob     float64                                `json:"logprob,omitempty"`
	TopLogprobs []ChatCompletionTokenLogprobTopLogprob `json:"top_logprobs"`
}

type ChatCompletionTokenLogprobTopLogprob struct {
	Token   string  `json:"token"`
	Bytes   []int64 `json:"bytes"`
	Logprob float64 `json:"logprob"`
}

type ChatCompletionStreamChoice struct {
	Index                int                                 `json:"index"`
	Delta                ChatCompletionStreamChoiceDelta     `json:"delta"`
	Logprobs             *ChatCompletionStreamChoiceLogprobs `json:"logprobs,omitempty"`
	FinishReason         FinishReason                        `json:"finish_reason"`
	ContentFilterResults ContentFilterResults                `json:"content_filter_results,omitempty"`
}

type PromptFilterResult struct {
	Index                int                  `json:"index"`
	ContentFilterResults ContentFilterResults `json:"content_filter_results,omitempty"`
}

type ChatCompletionStreamResponse struct {
	ID                  string                       `json:"id"`
	Object              string                       `json:"object"`
	Created             int64                        `json:"created"`
	Model               string                       `json:"model"`
	Choices             []ChatCompletionStreamChoice `json:"choices"`
	SystemFingerprint   string                       `json:"system_fingerprint"`
	PromptAnnotations   []PromptAnnotation           `json:"prompt_annotations,omitempty"`
	PromptFilterResults []PromptFilterResult         `json:"prompt_filter_results,omitempty"`
	// An optional field that will only be present when you set stream_options: {"include_usage": true} in your request.
	// When present, it contains a null value except for the last chunk which contains the token usage statistics
	// for the entire request.
	Usage *Usage `json:"usage,omitempty"`
}

// ChatStreamReader is an interface for reading chat completion streams.
type ChatStreamReader interface {
	Recv() (ChatCompletionStreamResponse, error)
	Close() error
}

// ChatCompletionStream
// Note: Perhaps it is more elegant to abstract Stream using generics.
type ChatCompletionStream struct {
	reader ChatStreamReader
}

// NewChatCompletionStream allows injecting a custom ChatStreamReader (for testing).
func NewChatCompletionStream(reader ChatStreamReader) *ChatCompletionStream {
	return &ChatCompletionStream{reader: reader}
}

// CreateChatCompletionStream — API call to create a chat completion w/ streaming
// support. It sets whether to stream back partial progress. If set, tokens will be
// sent as data-only server-sent events as they become available, with the
// stream terminated by a data: [DONE] message.
func (c *Client) CreateChatCompletionStream(
	ctx context.Context,
	request ChatCompletionRequest,
) (stream *ChatCompletionStream, err error) {
	urlSuffix := chatCompletionsSuffix
	if !checkEndpointSupportsModel(urlSuffix, request.Model) {
		err = ErrChatCompletionInvalidModel
		return
	}

	request.Stream = true
	reasoningValidator := NewReasoningValidator()
	if err = reasoningValidator.Validate(request); err != nil {
		return
	}

	req, err := c.newRequest(
		ctx,
		http.MethodPost,
		c.fullURL(urlSuffix, withModel(request.Model)),
		withBody(request),
	)
	if err != nil {
		return nil, err
	}

	resp, err := sendRequestStream[ChatCompletionStreamResponse](c, req)
	if err != nil {
		return
	}
	stream = &ChatCompletionStream{
		reader: resp,
	}
	return
}

func (s *ChatCompletionStream) Recv() (ChatCompletionStreamResponse, error) {
	return s.reader.Recv()
}

func (s *ChatCompletionStream) Close() error {
	return s.reader.Close()
}

func (s *ChatCompletionStream) Header() http.Header {
	if h, ok := s.reader.(interface{ Header() http.Header }); ok {
		return h.Header()
	}
	return http.Header{}
}

func (s *ChatCompletionStream) GetRateLimitHeaders() map[string]interface{} {
	if h, ok := s.reader.(interface{ GetRateLimitHeaders() RateLimitHeaders }); ok {
		headers := h.GetRateLimitHeaders()
		return map[string]interface{}{
			"x-ratelimit-limit-requests":     headers.LimitRequests,
			"x-ratelimit-limit-tokens":       headers.LimitTokens,
			"x-ratelimit-remaining-requests": headers.RemainingRequests,
			"x-ratelimit-remaining-tokens":   headers.RemainingTokens,
			"x-ratelimit-reset-requests":     headers.ResetRequests.String(),
			"x-ratelimit-reset-tokens":       headers.ResetTokens.String(),
		}
	}
	return map[string]interface{}{}
}
