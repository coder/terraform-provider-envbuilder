package tfutil

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/coder/envbuilder/log"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// TFValueToString converts an attr.Value to its string representation
// based on its Terraform type. This is needed because the String()
// method on an attr.Value creates a 'human-readable' version of the type, which
// leads to quotes, escaped characters, and other assorted sadness.
func TFValueToString(val attr.Value) string {
	if val.IsUnknown() || val.IsNull() {
		return ""
	}
	if vs, ok := val.(interface{ ValueString() string }); ok {
		return vs.ValueString()
	}
	if vb, ok := val.(interface{ ValueBool() bool }); ok {
		return fmt.Sprintf("%t", vb.ValueBool())
	}
	if vi, ok := val.(interface{ ValueInt64() int64 }); ok {
		return fmt.Sprintf("%d", vi.ValueInt64())
	}
	panic(fmt.Errorf("tfValueToString: value %T is not a supported type", val))
}

// TFListToStringSlice converts a types.List to a []string by calling
// tfValueToString on each element.
func TFListToStringSlice(l types.List) []string {
	els := l.Elements()
	ss := make([]string, len(els))
	for idx, el := range els {
		ss[idx] = TFValueToString(el)
	}
	return ss
}

// TFMapToStringMap converts a types.Map to a map[string]string by calling
// tfValueToString on each element.
func TFMapToStringMap(m types.Map) map[string]string {
	els := m.Elements()
	res := make(map[string]string, len(els))
	for k, v := range els {
		res[k] = TFValueToString(v)
	}
	return res
}

// TFLogFunc is an adapter to envbuilder/log.Func.
func TFLogFunc(ctx context.Context) log.Func {
	return func(level log.Level, format string, args ...any) {
		var logFn func(context.Context, string, ...map[string]interface{})
		switch level {
		case log.LevelTrace:
			logFn = tflog.Trace
		case log.LevelDebug:
			logFn = tflog.Debug
		case log.LevelWarn:
			logFn = tflog.Warn
		case log.LevelError:
			logFn = tflog.Error
		default:
			logFn = tflog.Info
		}
		logFn(ctx, fmt.Sprintf(format, args...))
	}
}

// DockerEnv returns the keys and values of the map in the form "key=value"
// sorted by key in lexicographical order. This is the format expected by
// Docker and some other tools that consume environment variables.
func DockerEnv(m map[string]string) []string {
	pairs := make([]string, 0, len(m))
	var sb strings.Builder
	for k := range m {
		_, _ = sb.WriteString(k)
		_, _ = sb.WriteRune('=')
		_, _ = sb.WriteString(m[k])
		pairs = append(pairs, sb.String())
		sb.Reset()
	}
	sort.Strings(pairs)
	return pairs
}
