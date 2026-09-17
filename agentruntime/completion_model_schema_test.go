package agentruntime

import (
	"encoding/json"
	"github.com/airlockrun/goai/tool"
	"testing"
)

func TestModelSeesInlineCompletionOutputWhileDispatchStaysStrict(t *testing.T) {
	in, model := input(batch(complete("done")))
	result, err := Run(t.Context(), in)
	if err != nil || result.Reply == nil {
		t.Fatalf("run=%#v %v", result, err)
	}
	if len(model.DoStreamCalls) != 1 {
		t.Fatalf("model calls=%d", len(model.DoStreamCalls))
	}
	var schema map[string]any
	for _, definition := range model.DoStreamCalls[0].Tools {
		if definition.Name == "complete" {
			if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
				t.Fatal(err)
			}
		}
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("provider schema hides properties behind a union: %s", mustJSON(schema))
	}
	output, ok := properties["output"].(map[string]any)
	if !ok || output["type"] != "object" {
		t.Fatalf("output is not visibly typed: %#v", output)
	}
	if _, exists := schema["oneOf"]; exists {
		t.Fatal("model-facing completion keeps unsupported root union")
	}
	for _, args := range []string{`{"kind":"output","output":"{\"answer\":42}"}`, `{"kind":"output","output":{"answer":42},"question":"mixed"}`} {
		invalid, _ := input(batch(call("bad", "complete", args), complete("good")))
		got, err := Run(t.Context(), invalid)
		if err != nil || got.Reply == nil {
			t.Fatalf("recovery=%#v %v", got, err)
		}
		cp, err := invalid.Store.LoadCheckpoint(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if cp.Calls[0].Result.Parts[0].Tool.Outcome != "error" {
			t.Fatal("invalid completion bypassed strict dispatch")
		}
	}
}

func TestCompletionModelSchemaPreservesRecursiveReferences(t *testing.T) {
	in, _ := input()
	in.Definition.OutputSchema = json.RawMessage(`{"$defs":{"Node":{"type":"object","properties":{"value":{"type":"integer"},"next":{"anyOf":[{"$ref":"#/$defs/Node"},{"type":"null"}]}},"required":["value"],"additionalProperties":false}},"$ref":"#/$defs/Node"}`)
	set, _, err := tools(in, tool.New("run_js").Build())
	if err != nil {
		t.Fatal(err)
	}
	presentation, err := completionModelSchema(set["complete"].InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	for _, schema := range []json.RawMessage{presentation, set["complete"].InputSchema} {
		if err := validateJSON(schema, json.RawMessage(`{"kind":"output","output":{"value":1,"next":{"value":2,"next":null}}}`)); err != nil {
			t.Fatalf("recursive schema lost: %v", err)
		}
		if err := validateJSON(schema, json.RawMessage(`{"kind":"output","output":{"value":1,"next":{"value":"wrong"}}}`)); err == nil {
			t.Fatal("recursive field constraints lost")
		}
	}
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
