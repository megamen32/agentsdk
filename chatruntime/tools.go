package chatruntime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/airlockrun/agentsdk/capability"
	"github.com/airlockrun/agentsdk/jsexec"
	"github.com/airlockrun/goai/tool"
	"github.com/airlockrun/sol/bus"
)

type runtime struct {
	input        Input
	ctx          context.Context
	executor     jsexec.Session
	permissionMu sync.Mutex
	approvalUsed bool
}

const confirmationMarker = "[requires_user_confirmation]"
const mandatoryPermission = "capability_confirmation"

// Mandatory capability gates deliberately do not inherit Sol's whole-tool
// wildcard approval on resume. A script may compute different arguments when
// rerun; only the exact capability and arguments shown in the checkpoint pass.
func (r *runtime) confirmCapability(ctx context.Context, d capability.Definition, callID string, input json.RawMessage) error {
	if !strings.Contains(d.Description, confirmationMarker) {
		return nil
	}
	if !json.Valid(input) {
		return errors.New("invalid capability arguments")
	}
	decoder := json.NewDecoder(strings.NewReader(string(input)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return err
	}
	pattern := fmt.Sprintf("%s:%x", d.Path.ID(), sha256.Sum256(canonical))
	r.permissionMu.Lock()
	defer r.permissionMu.Unlock()
	pm := bus.NewPermissionManager(bus.BusFromContext(ctx))
	if resume := r.input.Resume; resume != nil && resume.ToolCallID == callID && r.input.Approved != nil && *r.input.Approved {
		if r.approvalUsed {
			// Re-suspending here would replay an already executed side effect.
			return tool.DeniedError{Reason: "Only one confirmed capability invocation is allowed per tool call. Use a new tool call for another operation."}
		}
		var approved bus.ErrPermissionNeeded
		raw, marshalErr := json.Marshal(resume.Data)
		if marshalErr == nil && json.Unmarshal(raw, &approved) == nil && approved.Permission == mandatoryPermission && approved.ToolCallID == callID && len(approved.Patterns) == 1 && approved.Patterns[0] == pattern {
			pm.AddRule(bus.PermissionRule{Permission: mandatoryPermission, Pattern: pattern, Action: "allow"})
		}
	}
	// Airlock's confirmation UI renders description, not arbitrary metadata.
	// Include the exact bound arguments in that visible approval text.
	description := d.Description + "\nArguments: " + string(canonical)
	err = pm.Ask(ctx, bus.PermissionRequest{Permission: mandatoryPermission, Patterns: []string{pattern}, ToolCallID: callID, Metadata: map[string]any{"capability_id": d.Path.ID(), "arguments": json.RawMessage(canonical), "description": description}})
	if err == nil {
		r.approvalUsed = true
	}
	return err
}

func validateCapabilities(defs []capability.Definition) error {
	ids, direct, js := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, d := range defs {
		if d.Path.Kind() < capability.Air || d.Path.Kind() > capability.MCP {
			return errors.New("chatruntime: invalid capability kind")
		}
		if d.Target != capability.App && d.Target != capability.Platform && d.Target != capability.Executor {
			return errors.New("chatruntime: invalid capability target")
		}
		if d.Path.CanonicalOperation() == "" || ids[d.Path.ID()] || direct[d.Path.Direct()] || (d.Path.JS() != "" && js[d.Path.JS()]) || !json.Valid(d.InputSchema) {
			return fmt.Errorf("chatruntime: duplicate or invalid capability %s", d.Path.ID())
		}
		ids[d.Path.ID()], direct[d.Path.Direct()], js[d.Path.JS()] = true, true, true
	}
	return nil
}

func (r *runtime) tools() tool.Set {
	set := tool.Set{}
	if r.input.DirectTools {
		for _, d := range r.input.Capabilities {
			if d.Target == capability.Executor {
				continue
			}
			t := tool.Tool{Name: d.Path.Direct(), Description: d.Description, InputSchema: d.InputSchema, OutputSchema: d.OutputSchema}
			if d.LLMHint != "" {
				t.Description += "\n" + d.LLMHint
			}
			for _, ex := range d.InputExamples {
				t.InputExamples = append(t.InputExamples, tool.ToolInputExample{Input: ex})
			}
			t.Execute = func(ctx context.Context, input json.RawMessage, opts tool.CallOptions) (tool.Result, error) {
				if err := r.confirmCapability(ctx, d, opts.ToolCallID, input); err != nil {
					return tool.Result{}, err
				}
				return r.input.Backend.Invoke(ctx, Invocation{CapabilityID: d.Path.ID(), ToolCallID: opts.ToolCallID, Input: input})
			}
			set[t.Name] = t
		}
		return set
	}
	set["run_js"] = tool.New("run_js").
		Description("Execute an async JavaScript function body. Await capability calls; explicitly return the result. Supply a short description of the effect. Request confirmation for external side effects that need user approval.").
		SchemaFromStruct(struct {
			Code                string `json:"code"`
			Description         string `json:"description"`
			RequestConfirmation bool   `json:"request_confirmation,omitempty"`
		}{}).
		Execute(func(ctx context.Context, input json.RawMessage, opts tool.CallOptions) (tool.Result, error) {
			var args struct {
				Code                string `json:"code"`
				Description         string `json:"description"`
				RequestConfirmation bool   `json:"request_confirmation"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return tool.Result{}, err
			}
			if strings.TrimSpace(args.Code) == "" || strings.TrimSpace(args.Description) == "" {
				return tool.Result{}, errors.New("code and description are required")
			}
			if args.RequestConfirmation && !r.input.AutoConfirm {
				pm := bus.PermissionManagerFromContext(ctx)
				if pm == nil {
					return tool.Result{}, errors.New("permission manager is required")
				}
				if err := pm.Ask(ctx, bus.PermissionRequest{Permission: "run_js", Patterns: []string{"*"}, ToolCallID: opts.ToolCallID, Metadata: map[string]any{"code": args.Code, "description": args.Description}}); err != nil {
					var denied *bus.PermissionDeniedError
					if errors.As(err, &denied) {
						return tool.Result{}, tool.DeniedError{Reason: "Code execution was denied by the user."}
					}
					return tool.Result{}, err
				}
			}
			if err := ctx.Err(); err != nil {
				return tool.Result{}, err
			}
			if r.executor == nil {
				executor, err := r.input.ExecutorFactory(r.ctx, r.input.Capabilities)
				if err != nil {
					return tool.Result{}, err
				}
				if executor == nil {
					return tool.Result{}, errors.New("executor factory returned nil session")
				}
				r.executor = executor
			}
			callback := &invoker{backend: r.input.Backend, callID: opts.ToolCallID, catalog: r.input.Capabilities, confirm: r.confirmCapability}
			result, err := r.executor.Execute(ctx, args.Code, callback)
			callback.mu.Lock()
			pending := callback.permissionErr
			callback.mu.Unlock()
			if pending != nil {
				return tool.Result{}, pending
			}
			var output strings.Builder
			for _, log := range result.Logs {
				fmt.Fprintf(&output, "[%s] %s\n", log.Level, log.Message)
			}
			if !result.Undefined && len(result.Output) > 0 {
				if !json.Valid(result.Output) {
					return tool.Result{}, errors.New("executor returned invalid JSON")
				}
				output.Write(result.Output)
			}
			if err != nil && output.Len() != 0 {
				err = fmt.Errorf("%s\n%w", strings.TrimSpace(output.String()), err)
			}
			return tool.Result{Output: output.String(), Attachments: callback.attachments}, err
		}).Build()
	return set
}

// Each script receives a fresh immutable invoker, so retained JS callbacks
// cannot acquire a later tool call's attribution or attachments.
type invoker struct {
	backend       Backend
	callID        string
	catalog       []capability.Definition
	mu            sync.Mutex
	gateMu        sync.Mutex
	attachments   []tool.Attachment
	confirm       func(context.Context, capability.Definition, string, json.RawMessage) error
	permissionErr error
}

func (i *invoker) Invoke(ctx context.Context, id string, args json.RawMessage) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	i.mu.Lock()
	pending := i.permissionErr
	i.mu.Unlock()
	if pending != nil {
		return nil, pending
	}
	var selected *capability.Definition
	for _, d := range i.catalog {
		if d.Path.ID() == id && d.Target != capability.Executor {
			selected = &d
			break
		}
	}
	if selected == nil {
		return nil, errors.New("capability is not available in this run")
	}
	var positional []json.RawMessage
	if err := json.Unmarshal(args, &positional); err != nil || len(positional) != 1 {
		return nil, errors.New("capability requires one object argument")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(positional[0], &object); err != nil || object == nil {
		return nil, errors.New("capability requires one object argument")
	}
	if i.confirm != nil {
		// Serialize gate decisions with recording their result. Concurrent JS
		// calls must not execute after another callback requested suspension.
		i.gateMu.Lock()
		i.mu.Lock()
		pending := i.permissionErr
		i.mu.Unlock()
		if pending != nil {
			i.gateMu.Unlock()
			return nil, pending
		}
		if err := i.confirm(ctx, *selected, i.callID, positional[0]); err != nil {
			i.mu.Lock()
			if i.permissionErr == nil {
				i.permissionErr = err
			}
			i.mu.Unlock()
			i.gateMu.Unlock()
			return nil, err
		}
		i.gateMu.Unlock()
	}
	result, err := i.backend.Invoke(ctx, Invocation{CapabilityID: id, ToolCallID: i.callID, Input: positional[0]})
	i.mu.Lock()
	i.attachments = append(i.attachments, result.Attachments...)
	i.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if !selected.TextOutput && json.Valid([]byte(result.Output)) {
		return json.RawMessage(result.Output), nil
	}
	return json.Marshal(result.Output)
}
