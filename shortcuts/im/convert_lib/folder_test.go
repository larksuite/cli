package convertlib

// C4-C6：fetchFolderChildrenTree 单测（mock httpmock，不依赖真实 openapi）
// 覆盖 XML 一层输出（folder name+key+child_count / file name+key / 子文件夹 child_count / has_more）

import (
	"context"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func folderTestRuntime(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	rt := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+x"}, cfg, f, core.AsUser)
	return rt, reg
}

// C4：正常展开一层（文件 + 子文件夹 + child_count），无 has_more（items == all_count）
func TestFetchFolderChildrenTree_XMLOneLevel(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/im/v1/files/fld_root/folder",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"file_key": "f1", "name": "报告.pdf", "is_folder": false},
					map[string]interface{}{"file_key": "f2", "name": "文档.docx", "is_folder": false},
					map[string]interface{}{"file_key": "f3", "name": "子文件夹", "is_folder": true, "children_count": float64(3)},
				},
				"all_count": float64(3),
			},
		},
	})
	got := fetchFolderChildrenTree(rt, "fld_root", "tmpavatra", "om_123")
	want := `<folder name="tmpavatra" key="fld_root" child_count="3"><file name="报告.pdf" key="f1"/><file name="文档.docx" key="f2"/><folder name="子文件夹" key="f3" child_count="3"/></folder>`
	if got != want {
		t.Fatalf("fetchFolderChildrenTree() = %q, want %q", got, want)
	}
}

// C4b：items < all_count 时根 folder 带 has_more="true"
func TestFetchFolderChildrenTree_HasMore(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/im/v1/files/fld_root/folder",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"file_key": "f1", "name": "a.pdf", "is_folder": false},
				},
				"all_count": float64(100),
			},
		},
	})
	got := fetchFolderChildrenTree(rt, "fld_root", "big", "om_123")
	want := `<folder name="big" key="fld_root" child_count="100" has_more="true"><file name="a.pdf" key="f1"/></folder>`
	if got != want {
		t.Fatalf("fetchFolderChildrenTree() = %q, want %q", got, want)
	}
}

// C5：API 失败（error/nil）→ 返回空串（调用方降级旧输出）
func TestFetchFolderChildrenTree_APIFailure(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	// 不注册 stub → httpmock 返回错误
	got := fetchFolderChildrenTree(rt, "fld_root", "x", "om_123")
	if got != "" {
		t.Fatalf("fetchFolderChildrenTree() on API failure = %q, want empty (caller downgrades)", got)
	}
	_ = reg
}

// C6：items 空 → 返回空串（降级）
func TestFetchFolderChildrenTree_EmptyItems(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/im/v1/files/fld_root/folder",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":     []interface{}{},
				"all_count": float64(0),
			},
		},
	})
	got := fetchFolderChildrenTree(rt, "fld_root", "x", "om_123")
	if got != "" {
		t.Fatalf("fetchFolderChildrenTree() empty items = %q, want empty", got)
	}
}
