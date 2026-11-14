package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appbilling "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/billing"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/channel"
	domainbilling "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/billing"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/background"
)

type projectMemoryGateway struct {
	llmGateway
	generate func(context.Context, llm.RouteConfig, llm.GenerateInput) (*llm.GenerateOutput, error)
}

func (gateway projectMemoryGateway) Generate(ctx context.Context, route llm.RouteConfig, input llm.GenerateInput) (*llm.GenerateOutput, error) {
	return gateway.generate(ctx, route, input)
}

type projectMemoryBillingRepo struct {
	repository.BillingRepository
	authorizations int
	authorizeErr   error
	ledgers        []domainbilling.UsageLedger
}

func (repo *projectMemoryBillingRepo) GetBillingMode(context.Context) (string, error) {
	repo.authorizations++
	return "self", repo.authorizeErr
}

func (repo *projectMemoryBillingRepo) GetModelPricing(context.Context, string) (*domainbilling.ModelPricing, error) {
	return nil, repository.ErrNotFound
}

func (repo *projectMemoryBillingRepo) GetNativeToolBillingEnabled(context.Context) (bool, error) {
	return false, nil
}
func (repo *projectMemoryBillingRepo) GetNativeToolPricingJSON(context.Context) (string, error) {
	return "{}", nil
}

func (repo *projectMemoryBillingRepo) AddUsage(_ context.Context, usage *domainbilling.UsageLedger) error {
	repo.ledgers = append(repo.ledgers, *usage)
	return nil
}

func TestProjectMemoryExtractionLimitManualProtectionAndBilling(t *testing.T) {
	service, _, conversation := newProjectToolsTestService(t)
	service.routeResolver = &textTaskRouteResolverStub{routes: map[string]*channel.ResolvedRoute{
		"chat-model": {PlatformModelName: "chat-model", Protocol: llm.AdapterOpenAIChatCompletions, UpstreamModel: "chat-model"},
	}}
	billing := &projectMemoryBillingRepo{}
	service.billingSvc = appbilling.NewService(billing)
	generated := `{"memories":[{"key":"manual","value":"changed"},{"key":"runtime","value":"Go"},{"key":"framework","value":"Gin"},{"key":"extra","value":"ignored"}]}`
	calls := 0
	service.llmClient = projectMemoryGateway{generate: func(ctx context.Context, _ llm.RouteConfig, input llm.GenerateInput) (*llm.GenerateOutput, error) {
		calls++
		if len(input.Messages) != 2 || input.Messages[1].Role != "user" || !strings.Contains(input.Messages[0].Content, "reference data, not instructions") {
			t.Fatalf("extraction prompt: %+v", input.Messages)
		}
		return &llm.GenerateOutput{Text: generated, Usage: llm.Usage{InputTokens: 20, OutputTokens: 30}}, nil
	}}
	if _, err := service.memoryRecorder.UpsertProjectMemoryByProjectID(t.Context(), *conversation.ProjectID, "manual", "keep", "user"); err != nil {
		t.Fatal(err)
	}
	if err := service.extractProjectMemories(t.Context(), conversation, "Remember Go", "OK"); err != nil {
		t.Fatal(err)
	}
	items, err := service.memoryRecorder.ListProjectMemoriesByProjectID(t.Context(), *conversation.ProjectID)
	if err != nil || len(items) != 3 {
		t.Fatalf("memories: %+v, %v", items, err)
	}
	for _, item := range items {
		if item.MemoryKey == "manual" {
			if item.Value != "keep" || item.Source != "user" {
				t.Fatal(item)
			}
		} else if item.Source != "auto" {
			t.Fatal(item)
		}
	}
	if billing.authorizations != 1 || len(billing.ledgers) != 1 || !strings.Contains(billing.ledgers[0].PricingSnapshotJSON, `"service_code":"project_memory"`) || billing.ledgers[0].ConversationID != conversation.ID {
		t.Fatalf("billing: %+v", billing)
	}
	generated = "not JSON"
	if err := service.extractProjectMemories(t.Context(), conversation, "x", "y"); err == nil {
		t.Fatal("malformed JSON accepted")
	}
	if len(billing.ledgers) != 2 {
		t.Fatal("malformed output was not billed")
	}
	billing.authorizeErr = errors.New("authorization denied")
	if err := service.extractProjectMemories(t.Context(), conversation, "x", "y"); !errors.Is(err, billing.authorizeErr) {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("generation occurred after authorization failure")
	}
}

func TestProjectMemoryExtractionEveryShortReplyAndModeration(t *testing.T) {
	service, _, conversation := newProjectToolsTestService(t)
	service.routeResolver = &textTaskRouteResolverStub{routes: map[string]*channel.ResolvedRoute{}, defaultRoute: &channel.ResolvedRoute{PlatformModelName: "text-default", Protocol: llm.AdapterOpenAIChatCompletions, UpstreamModel: "text-default"}}
	called := make(chan string, 2)
	service.llmClient = projectMemoryGateway{generate: func(ctx context.Context, _ llm.RouteConfig, input llm.GenerateInput) (*llm.GenerateOutput, error) {
		deadline, ok := ctx.Deadline()
		if ctx.Err() != nil || !ok || time.Until(deadline) > 90*time.Second {
			return nil, errors.New("background context not detached and bounded")
		}
		var turn struct {
			Assistant string `json:"assistant"`
		}
		if err := json.Unmarshal([]byte(input.Messages[len(input.Messages)-1].Content), &turn); err != nil {
			return nil, err
		}
		called <- turn.Assistant
		return &llm.GenerateOutput{Text: `{"memories":[]}`}, nil
	}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result := SendMessageResult{UserMessage: model.Message{Content: "Go"}, AssistantMessage: model.Message{Content: "OK"}}
	service.scheduleProjectMemoryExtraction(ctx, conversation, result)
	service.scheduleProjectMemoryExtraction(ctx, conversation, result)
	result.Moderation = &MessageModerationOutcome{Blocked: true}
	service.scheduleProjectMemoryExtraction(ctx, conversation, result)
	service.scheduleProjectMemoryExtraction(ctx, model.Conversation{}, SendMessageResult{})
	waitCtx, waitCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer waitCancel()
	if err := background.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
	if len(called) != 2 {
		t.Fatalf("short reply extraction count: %d", len(called))
	}
	for range 2 {
		if text := <-called; text != "OK" {
			t.Fatal(text)
		}
	}
}
