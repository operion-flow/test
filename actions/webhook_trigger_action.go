package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	webhookPersistence "github.com/dukex/operion/pkg/providers/webhook/persistence"
	"github.com/operion-flow/test/integration/environment"
)

// WebhookTriggerAction executes a webhook trigger
type WebhookTriggerAction struct {
	SourceId string
	Payload  map[string]any
	Method   string
}

func (wta *WebhookTriggerAction) Execute(env *environment.Environment) error {
	persistence, err := webhookPersistence.NewPostgresPersistence(context.Background(), env.Logger, env.PostgresURL())
	if err != nil {
		return fmt.Errorf("failed to create webhook persistence: %w", err)
	}
	defer persistence.Close()

	source, err := persistence.WebhookSourceByID(wta.SourceId)
	if err != nil {
		return fmt.Errorf("failed to get webhook source by ID: %w", err)
	}
	if source == nil {
		return fmt.Errorf("webhook source not found for ID: %s", wta.SourceId)
	}

	url := env.WebhookURL() + "/" + source.ExternalID.String()
	method := wta.Method
	if method == "" {
		method = "POST"
	}

	var bodyReader *bytes.Buffer
	if wta.Payload != nil {
		jsonData, err := json.Marshal(wta.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonData)
	} else {
		bodyReader = bytes.NewBuffer([]byte{})
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if wta.Payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func (wta *WebhookTriggerAction) Description() string {
	return fmt.Sprintf("Execute webhook, method=%s source_id=%s", wta.Method, wta.SourceId)
}
