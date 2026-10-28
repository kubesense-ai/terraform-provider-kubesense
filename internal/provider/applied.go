package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// The server normalises some values it is sent: it trims a URL, re-encodes a
// JSON field, resolves a name. Comparing what the user wrote with what the
// server returns would then show a change on every plan.
//
// So after each write the provider records, in the resource's private state,
// what the server returned for each key ("applied"). On the next read a key is
// unchanged when the server still returns that same value, and the user's own
// string stays in state; only a value that moved since the last apply (an edit
// in the UI) is taken from the server and shows up as drift.

// privateGetter and privateSetter are the framework's private state API, as
// requests (read) and responses (write) expose it.
type privateGetter interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
}

type privateSetter interface {
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

func loadApplied(ctx context.Context, p privateGetter, key string, diags *diag.Diagnostics) map[string]string {
	raw, d := p.GetKey(ctx, key)
	diags.Append(d...)
	out := map[string]string{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func saveApplied(ctx context.Context, p privateSetter, key string, values map[string]string, diags *diag.Diagnostics) {
	raw, err := json.Marshal(values)
	if err != nil {
		diags.AddError("Saving private state failed", err.Error())
		return
	}
	diags.Append(p.SetKey(ctx, key, raw)...)
}

// reconcile decides each configured key's value in state after a read: the
// user's value when the server's is unchanged since the last apply, the
// server's when it moved. A key the server no longer has is dropped, so the
// plan shows it being set again.
func reconcile(prior, server, applied map[string]string) map[string]string {
	out := make(map[string]string, len(prior))
	for k, mine := range prior {
		theirs, ok := server[k]
		if !ok {
			continue
		}
		if theirs == mine || applied[k] == theirs {
			out[k] = mine
		} else {
			out[k] = theirs
		}
	}
	return out
}
