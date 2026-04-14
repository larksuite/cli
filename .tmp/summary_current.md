# CLI测试

## Round-trip 测试记录

<lark-table rows="6" cols="6" header-row="true" column-widths="100,220,200,180,80,220">

  <lark-tr>
    <lark-td>
      **测试时间**
    </lark-td>
    <lark-td>
      **工具 / 版本**
    </lark-td>
    <lark-td>
      **源文档**
    </lark-td>
    <lark-td>
      **目标文档**
    </lark-td>
    <lark-td>
      **Diff**
    </lark-td>
    <lark-td>
      **备注**
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      2026-04-02
    </lark-td>
    <lark-td>
      内部版 `github/cli`

      `docs +export` → `docs +update`

      （有 `fixExportedMarkdown`）
    </lark-td>
    <lark-td>
      [Harness Engineering](https://bytedance.larkoffice.com/wiki/SRZ0wiK0PilSMskPiiLcd9jjnAc)（427行）
    </lark-td>
    <lark-td>
      [【导入测试】Harness Engineering](https://bytedance.larkoffice.com/wiki/WzqewEHoWi27NtkILYdcB0rgnng)
    </lark-td>
    <lark-td>
      **1 行**
    </lark-td>
    <lark-td>
      仅图片 token 重新生成（正常），准确率 ≈ 99.8%
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      2026-04-02
    </lark-td>
    <lark-td>
      开源版 `lark-cli` main

      `docs +fetch` → `docs +create`

      （**无**后处理）
    </lark-td>
    <lark-td>
      [Harness Engineering](https://bytedance.larkoffice.com/wiki/SRZ0wiK0PilSMskPiiLcd9jjnAc)（303行，原始输出）
    </lark-td>
    <lark-td>
      [【lark-cli 开源版测试】Harness Engineering](https://www.feishu.cn/wiki/CSCtwH66oiw8YCkW8PFcEXRPnhb)
    </lark-td>
    <lark-td>
      **166 行**
    </lark-td>
    <lark-td>
      段落全部折叠，根因：缺少 `fixExportedMarkdown()` 后处理
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      2026-04-02
    </lark-td>
    <lark-td>
      开源版 `lark-cli` feat/docs-export-import-fix

      `docs +fetch` → `docs +update`

      （**迁移后处理后**）
    </lark-td>
    <lark-td>
      [Harness Engineering](https://bytedance.larkoffice.com/wiki/SRZ0wiK0PilSMskPiiLcd9jjnAc)（427行）
    </lark-td>
    <lark-td>
      [【lark-cli 开源版测试】Harness Engineering](https://www.feishu.cn/wiki/CSCtwH66oiw8YCkW8PFcEXRPnhb)
    </lark-td>
    <lark-td>
      **1 行**
    </lark-td>
    <lark-td>
      与内部版一致，准确率 ≈ 99.8%，修复验证通过 ✅
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      2026-04-02
    </lark-td>
    <lark-td>
      开源版 `lark-cli` feat/docs-export-import-fix

      `docs +fetch` → `docs +update`

      （含 callout/quote-container 修复）
    </lark-td>
    <lark-td>
      [Claude Code 封号机制逆向探查](https://bytedance.larkoffice.com/docx/E2JudVzf7oCNfhxyxaQcZIW1n0g)（839行，含画板/callout）
    </lark-td>
    <lark-td>
      [【导入测试】Claude Code 封号机制逆向探查](https://www.feishu.cn/wiki/SbXgwxLh4i7O1pkuMfGcz9PWnXc)
    </lark-td>
    <lark-td>
      **19 行**
    </lark-td>
    <lark-td>
      剩余：whiteboard 丢失（4处，create-doc 设计限制）+ emoji ⚠️→🎁（MCP 服务端 bug）
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      2026-04-02
    </lark-td>
    <lark-td>
      开源版 `lark-cli` feat/docs-export-import-fix

      `docs +fetch` → `docs +update`

      （+ fixCodeBlockTrailingBlanks + 嵌套围栏修复）
    </lark-td>
    <lark-td>
      [Lark CLI 开发维护手册](https://bytedance.larkoffice.com/docx/NoRqday2tohtQ4xKzIdczj5envd)（2055行，含有序列表/callout/表格/图片/reference-synced）
    </lark-td>
    <lark-td>
      [【导入测试】Lark CLI 开发维护手册](https://www.feishu.cn/wiki/YIlMwWvLSiSzqykbTfCc87sanVe)
    </lark-td>
    <lark-td>
      **156 行**
    </lark-td>
    <lark-td>
      CLI 层修复：代码块尾部多余空行（-26行）+ 嵌套围栏跟踪混乱（-18行）。剩余均为 MCP 层问题：① 有序列表编号重置为 1（create-doc bug）② emoji ⚠️→🎁（fetch-doc bug）③ reference-synced/unsupported 块丢失 ④ 图片 token 失效（源文档权限）
    </lark-td>
  </lark-tr>
</lark-table>

