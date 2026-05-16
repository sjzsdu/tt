package picoclaw

import (
	"fmt"
	"strings"
)

const emptyResponseSentinel = "The model returned an empty response. This may indicate a provider error or token limit."

func normalizeDirectResponse(resp string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(resp)
	if trimmed == "" {
		return "", fmt.Errorf("model returned an empty response")
	}
	if trimmed == emptyResponseSentinel {
		return "", fmt.Errorf("model returned an empty response (provider may have returned no final content)")
	}
	return resp, nil
}
