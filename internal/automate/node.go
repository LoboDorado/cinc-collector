package automate

import "encoding/json"

// NodeFields is the subset of a saved Chef node document (the
// run_converge message's "node" field, i.e. `node.data_for_save`) that
// Cinc Collector surfaces directly on its own nodes table. Everything else
// in the node document is kept opaquely in Node.Attributes by the caller.
//
// Extraction is defensive: a missing or oddly-shaped field just leaves the
// corresponding result field empty rather than failing, since this data
// comes from Ohai (automatic attributes) and varies across platforms.
type NodeFields struct {
	FQDN            string
	Platform        string
	PlatformFamily  string
	PlatformVersion string
	ChefVersion     string
	Environment     string
}

// ExtractNodeFields pulls well-known fields out of a saved Chef node
// document's JSON. raw may be nil or empty (e.g. when the Chef server
// doesn't attach node state to the message), in which case it returns the
// zero value.
func ExtractNodeFields(raw json.RawMessage) NodeFields {
	var out NodeFields
	if len(raw) == 0 {
		return out
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return out
	}

	out.Environment, _ = doc["chef_environment"].(string)

	automatic, _ := doc["automatic"].(map[string]any)
	if automatic == nil {
		return out
	}
	out.FQDN, _ = automatic["fqdn"].(string)
	out.Platform, _ = automatic["platform"].(string)
	out.PlatformFamily, _ = automatic["platform_family"].(string)
	out.PlatformVersion, _ = asString(automatic["platform_version"])

	if chefPackages, ok := automatic["chef_packages"].(map[string]any); ok {
		if chef, ok := chefPackages["chef"].(map[string]any); ok {
			out.ChefVersion, _ = chef["version"].(string)
		}
	}
	return out
}

// asString handles values Ohai sometimes reports as numbers (e.g. some
// platform_version values on certain platforms) as well as strings.
func asString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case float64:
		return jsonNumberString(t), true
	default:
		return "", false
	}
}

func jsonNumberString(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}
