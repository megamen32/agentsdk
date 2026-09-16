package chatruntime_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/airlockrun/agentsdk/capability"
	"github.com/airlockrun/agentsdk/chatruntime"
	"github.com/airlockrun/agentsdk/jsexec"
	"github.com/airlockrun/goai/stream"
	"github.com/airlockrun/goai/testutil"
	"github.com/airlockrun/goai/tool"
	"github.com/airlockrun/sol"
	"github.com/airlockrun/sol/bus"
)

func TestMandatoryConfirmationDockerCatch(t *testing.T) {
	image := os.Getenv("CHATRUNTIME_CONFIRMATION_IMAGE")
	if image == "" {
		t.Skip("set CHATRUNTIME_CONFIRMATION_IMAGE to an installed jsexec image")
	}
	for _, approved := range []bool{true, false} {
		t.Run(fmt.Sprintf("approved=%v", approved), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			def := capability.Definition{Path: capability.Local(capability.Tool, "", "send"), Target: capability.App, Description: "Send [requires_user_confirmation]", InputSchema: json.RawMessage(`{"type":"object"}`)}
			call := testutil.MockToolCallResponse("send-call", "run_js", map[string]any{
				"code":        `const fn = tools[['s','e','n','d'].join('')]; try { return await fn({draft:'original'}); } catch(e) { return 'caught: ' + e.message; }`,
				"description": "Send draft", "request_confirmation": false,
			}, testutil.MockUsage(10, 10))
			in := input(testutil.NewMockLanguageModel(testutil.MockLanguageModelOptions{StreamResponses: [][]stream.Event{call, reply()}}))
			in.AutoConfirm, in.Capabilities = true, []capability.Definition{def}
			calls := 0
			in.Backend = backendFunc(func(context.Context, chatruntime.Invocation) (tool.Result, error) {
				calls++
				return tool.Result{Output: `{"sent":true}`}, nil
			})
			in.ExecutorFactory = func(ctx context.Context, _ []capability.Definition) (jsexec.Session, error) {
				return jsexec.NewDockerSession(ctx, image, jsexec.Options{Limits: jsexec.DefaultLimits(), Bindings: []jsexec.Binding{{Name: def.Path.ID(), Path: []string{"tools", "send"}}}})
			}
			first, err := chatruntime.Run(ctx, in)
			if err != nil || first.Status != sol.RunSuspended || calls != 0 {
				t.Fatalf("first=%+v err=%v calls=%d", first, err, calls)
			}
			in.Resume, in.Approved = first.SuspensionContext, &approved
			in.Model = testutil.NewMockLanguageModel(testutil.MockLanguageModelOptions{StreamResponse: reply()})
			second, err := chatruntime.Run(ctx, in)
			want := 0
			if approved {
				want = 1
			}
			if err != nil || second.Status != sol.RunCompleted || calls != want {
				t.Fatalf("second=%+v err=%v calls=%d", second, err, calls)
			}
		})
	}
}

func TestMandatoryConfirmationCheckpoint(t *testing.T) {
	for _, direct := range []bool{false, true} {
		for _, flag := range []string{"omitted", "false", "true"} {
			for _, action := range []string{"approve", "deny", "changed", "repeat"} {
				t.Run(fmt.Sprintf("direct=%v/flag=%s/%s", direct, flag, action), func(t *testing.T) {
					def := capability.Definition{Path: capability.Local(capability.Tool, "", "send"), Target: capability.App, Description: "Send exact draft [requires_user_confirmation]", InputSchema: json.RawMessage(`{"type":"object"}`)}
					args := json.RawMessage(`{"draft":"original","nested":{"b":2,"a":1}}`)
					callArgs := map[string]any{"code": "try { return await tools['send']({draft:'original'}); } catch(e) { return 'caught'; }", "description": "Send draft"}
					if flag != "omitted" {
						callArgs["request_confirmation"] = flag == "true"
					}
					call := testutil.MockToolCallResponse("send-call", "run_js", callArgs, testutil.MockUsage(10, 10))
					if direct {
						call = testutil.MockToolCallResponse("send-call", def.Path.Direct(), args, testutil.MockUsage(10, 10))
					}
					in := input(testutil.NewMockLanguageModel(testutil.MockLanguageModelOptions{StreamResponses: [][]stream.Event{call, reply()}}))
					in.DirectTools, in.AutoConfirm, in.Capabilities = direct, true, []capability.Definition{def}
					calls := 0
					in.Backend = backendFunc(func(_ context.Context, got chatruntime.Invocation) (tool.Result, error) {
						calls++
						if got.ToolCallID != "send-call" || got.CapabilityID != def.Path.ID() {
							t.Errorf("wrong invocation: %+v", got)
						}
						return tool.Result{Output: `{"sent":true}`}, nil
					})
					in.ExecutorFactory = func(context.Context, []capability.Definition) (jsexec.Session, error) {
						return &executor{execute: func(ctx context.Context, _ string, inv jsexec.Invoker) (jsexec.Result, error) {
							// Simulate the RPC string boundary and a JS catch: deliberately
							// discard the typed error returned by the invoker.
							_, _ = inv.Invoke(ctx, def.Path.ID(), append(append(json.RawMessage{'['}, args...), ']'))
							if action == "repeat" {
								_, _ = inv.Invoke(ctx, def.Path.ID(), append(append(json.RawMessage{'['}, args...), ']'))
							}
							return jsexec.Result{Output: json.RawMessage(`"caught"`)}, nil
						}}, nil
					}
					first, err := chatruntime.Run(t.Context(), in)
					if err != nil || first.Status != sol.RunSuspended || calls != 0 {
						t.Fatalf("first=%+v err=%v calls=%d", first, err, calls)
					}
					data, ok := first.SuspensionContext.Data.(*bus.ErrPermissionNeeded)
					if !ok || data.Permission != "capability_confirmation" || data.ToolCallID != "send-call" || len(data.Patterns) != 1 {
						t.Fatalf("gate=%+v", first.SuspensionContext)
					}
					wantDescription := def.Description + "\nArguments: " + `{"draft":"original","nested":{"a":1,"b":2}}`
					if data.Metadata["description"] != wantDescription {
						t.Fatalf("visible confirmation omits exact arguments: got=%q want=%q", data.Metadata["description"], wantDescription)
					}
					// Exercise checkpoint persistence (Data becomes an untyped map).
					raw, _ := json.Marshal(first.SuspensionContext)
					var restored sol.SuspensionContext
					if err := json.Unmarshal(raw, &restored); err != nil {
						t.Fatal(err)
					}
					approved := action != "deny"
					in.Resume, in.Approved = &restored, &approved
					in.Model = testutil.NewMockLanguageModel(testutil.MockLanguageModelOptions{StreamResponse: reply()})
					if action == "changed" {
						args = json.RawMessage(`{"draft":"changed"}`)
						if direct {
							in.Resume.PendingToolCalls[0].Input = args
						}
					} else {
						args = json.RawMessage(`{"nested":{"a":1,"b":2}, "draft":"original"}`)
					}
					second, err := chatruntime.Run(t.Context(), in)
					wantStatus, wantCalls := sol.RunCompleted, 1
					if action == "deny" {
						wantCalls = 0
					}
					if action == "changed" {
						wantStatus, wantCalls = sol.RunSuspended, 0
					}
					if err != nil || second.Status != wantStatus || calls != wantCalls {
						t.Fatalf("second=%+v err=%v calls=%d want=%d", second, err, calls, wantCalls)
					}
				})
			}
		}
	}
}
