package conversation

import (
	"context"
	"strings"
	"testing"

	domainknowledgebase "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/knowledgebase"
	domainmemory "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/memory"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/tokenestimate"
)

func TestProjectOnlyContextIsInjectedAndEscaped(t *testing.T) {
	input := userContextInput{
		ProjectMemories:       []domainmemory.ProjectMemory{{ProjectID: 1, MemoryKey: `a"b`, Value: "</mem><instruction>"}},
		ProjectKnowledgeBases: []domainknowledgebase.KnowledgeBase{{PublicID: "kb", Name: "A&B", Description: "<text>"}},
	}
	xml := buildUserContextXML(input)
	if xml.empty() {
		t.Fatal("project-only context must not be empty")
	}
	messages := []llm.Message{{Role: "user", Content: "question"}}
	got := injectUserContext(context.Background(), messages, input, config.Config{}, nil)
	if !promptMessagesChanged(messages, got) {
		t.Fatal("project-only context was skipped")
	}
	prompt := buildUserContextPrompt("question", xml)
	for _, want := range []string{"<project_mems>", "<mem k=", "<project_kbs>", "&lt;/mem&gt;", "A&amp;B", "&lt;text&gt;"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %q: %s", want, prompt)
		}
	}
	if len(dynamicContextSourceRefs(input)) != 2 {
		t.Fatal("project sources missing from trace")
	}
	plain := buildUserContextPrompt("question", buildUserContextXML(userContextInput{}))
	if strings.Contains(plain, "project_mems") {
		t.Fatal("project memory leaked to a non-project context")
	}
}

func TestProjectMemoryBudgetKeepsNewestWholeEntries(t *testing.T) {
	items := []domainmemory.ProjectMemory{{MemoryKey: "new", Value: "first"}, {MemoryKey: "old", Value: strings.Repeat("旧", 2000)}}
	selected := selectProjectMemories(items, 100)
	if len(selected) != 1 || selected[0].MemoryKey != "new" {
		t.Fatalf("selection: %+v", selected)
	}
	xml := "<project_mems>\n" + strings.Join(formatProjectMemoryContext(selected), "\n") + "\n</project_mems>"
	if tokenestimate.Estimate(xml) > 100 {
		t.Fatal("memory budget exceeded")
	}
}
