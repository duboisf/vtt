package openai

import (
	"context"
	"strings"
	"time"

	openaisdk "github.com/openai/openai-go/v3"

	"vocis/internal/config"
	"vocis/internal/sessionlog"
)

const postProcessTimeout = 5 * time.Second

func (c *Client) PostProcess(ctx context.Context, cfg config.PostProcessConfig, text string) string {
	if !cfg.Enabled || strings.TrimSpace(text) == "" {
		return text
	}

	ctx, cancel := context.WithTimeout(ctx, postProcessTimeout)
	defer cancel()

	resp, err := c.client.Chat.Completions.New(ctx, openaisdk.ChatCompletionNewParams{
		Model: openaisdk.ChatModel(cfg.Model),
		Messages: []openaisdk.ChatCompletionMessageParamUnion{
			openaisdk.SystemMessage(cfg.Prompt),
			openaisdk.UserMessage(text),
		},
	})
	if err != nil {
		sessionlog.Warnf("postprocess failed, using raw transcription: %v", err)
		return text
	}

	if len(resp.Choices) == 0 {
		sessionlog.Warnf("postprocess returned no choices, using raw transcription")
		return text
	}

	cleaned := strings.TrimSpace(resp.Choices[0].Message.Content)
	if cleaned == "" {
		sessionlog.Warnf("postprocess returned empty text, using raw transcription")
		return text
	}

	sessionlog.Infof("postprocess cleaned=%d raw=%d model=%s", len(cleaned), len(text), cfg.Model)
	return cleaned
}
