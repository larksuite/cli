package doc

import (
	"context"
	"net/url"
	"time"

	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// DocsRead uses direct read-only APIs, never MCP or an implicit authorization flow.
var DocsRead = common.Shortcut{
	Service: "docs", Command: "+read", Description: "Read document body and optional embedded sheet values through direct APIs", Risk: "read", Scopes: []string{"docx:document:readonly"}, AuthTypes: []string{"bot", "user"}, HasFormat: true,
	Flags: []common.Flag{{Name: "document-host", Desc: "explicit exact document-link hostname allowlist for this invocation (API host unchanged)"}, {Name: "doc", Desc: "configured-tenant docx/wiki URL or docx token", Required: true}, {Name: "full", Type: "bool", Desc: "include all blocks and recognized embedded sheet values"}, {Name: "max-pages", Type: "int", Default: "100", Desc: "maximum blocks pages"}, {Name: "max-cells", Type: "int", Default: "100000", Desc: "maximum embedded sheet cells"}},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ep, err := runtime.Config.ResolveServiceEndpoints()
		if err != nil {
			return err
		}
		hosts := []string{}
		for _, base := range []string{ep.Open, ep.Accounts} {
			if parsed, e := url.Parse(base); e == nil {
				hosts = append(hosts, parsed.Host)
			}
		}
		bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		apiClient, err := runtime.Factory.NewAPIClientWithConfig(runtime.Config)
		if err != nil {
			return err
		}
		result, err := ReadDocument(bounded, DocReadOptions{DocumentHost: runtime.Str("document-host"), Input: runtime.Str("doc"), AllowedHosts: hosts, Identity: string(runtime.As()), Full: runtime.Bool("full"), MaxPages: runtime.Int("max-pages"), MaxCells: runtime.Int("max-cells")}, func(ctx context.Context, path string, params map[string]interface{}) (map[string]interface{}, error) {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			value, e := apiClient.CallAPI(ctx, client.RawApiRequest{Method: "GET", URL: path, Params: params, As: runtime.As()})
			if e != nil {
				return nil, e
			}
			if e = client.CheckLarkResponse(value); e != nil {
				return nil, e
			}
			envelope, ok := value.(map[string]interface{})
			if !ok {
				return nil, output.ErrNetwork("invalid document response")
			}
			data, ok := envelope["data"].(map[string]interface{})
			if !ok {
				return nil, output.ErrNetwork("missing document data")
			}
			return data, nil
		})
		// Partial material is retained on stdout, and the top-level handler emits one stderr error.
		if result != nil {
			if err == nil {
				runtime.Out(result, nil)
			} else if result.bodyRead || len(result.Sheets) > 0 {
				output.PrintJson(runtime.IO().Out, map[string]interface{}{"ok": false, "partial": true, "identity": string(runtime.As()), "data": result})
			}
		}
		if err != nil {
			return output.Errorf(output.ExitAPI, "document_read_error", "%v", err)
		}
		return nil
	},
}
