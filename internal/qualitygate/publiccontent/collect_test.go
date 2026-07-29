// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package publiccontent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/testutil/gitcmd"
)

func TestCollectScansOnlyCurrentContributionAndMetadata(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")

	writeFile(t, filepath.Join(repo, "baseline.md"), `BASE_`+`TOKEN="baseline-only"
`)
	runGit(t, repo, "add", "baseline.md")
	runGit(t, repo, "commit", "-m", "base")

	providerValue := "ghp_" + "1234567890abcdef1234567890abcdef1234"
	writeFile(t, filepath.Join(repo, "docs", "public.md"), `# Public change

api_`+`key = "`+providerValue+`"
`)
	runGit(t, repo, "add", "docs/public.md")
	runGit(t, repo, "commit", "-m", "add public doc", "-m", "Change"+"-Id: I0123456789abcdef0123456789abcdef01234567")

	metadataPath := filepath.Join(repo, "pr-metadata.json")
	writeFile(t, metadataPath, `{"title":"publish public docs","body":"Reviewed`+`-on: https://review.example.test/c/project/+/123"}`)

	got, err := Collect(context.Background(), Options{
		Repo:         repo,
		ChangedFrom:  "HEAD~1",
		MetadataPath: metadataPath,
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	rules := findingRules(got)
	for _, want := range []string{
		"public_content_generic_credential",
		"public_content_change_id_trailer",
		"public_content_reviewed_on_trailer",
	} {
		if !rules[want] {
			t.Fatalf("missing rule %s in findings %#v", want, got)
		}
	}
	for _, item := range got {
		if item.File == "baseline.md" {
			t.Fatalf("collector scanned unchanged baseline file: %#v", got)
		}
	}
}

func TestCollectScansOnlyChangedLinesInChangedFiles(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")

	writeFile(t, filepath.Join(repo, "docs", "workflow.md"), "SECRET_TOKEN=legacy-example\npublic baseline\n")
	runGit(t, repo, "add", "docs/workflow.md")
	runGit(t, repo, "commit", "-m", "base")

	writeFile(t, filepath.Join(repo, "docs", "workflow.md"), "SECRET_TOKEN=legacy-example\npublic baseline\nnew public line\n")
	runGit(t, repo, "add", "docs/workflow.md")
	runGit(t, repo, "commit", "-m", "add public line")

	metadataPath := filepath.Join(repo, "pr-metadata.json")
	writeFile(t, metadataPath, `{}`)

	got, err := Collect(context.Background(), Options{
		Repo:         repo,
		ChangedFrom:  "HEAD~1",
		MetadataPath: metadataPath,
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	for _, item := range got {
		if item.Rule == "public_content_generic_credential" && item.File == "docs/workflow.md" {
			t.Fatalf("collector scanned unchanged legacy line in changed file: %#v", got)
		}
	}
}

func TestCollectSemanticCandidatesStoreSanitizedReviewText(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")

	writeFile(t, filepath.Join(repo, "docs", "public.md"), "base\n")
	runGit(t, repo, "add", "docs/public.md")
	runGit(t, repo, "commit", "-m", "base")

	raw := "private launch plan for alpha-service rollout on Friday with SERVICE_" + "TOKEN=real-" + "secret-value"
	writeFile(t, filepath.Join(repo, "docs", "public.md"), "base\n"+raw+"\n")
	runGit(t, repo, "add", "docs/public.md")
	runGit(t, repo, "commit", "-m", "add semantic candidate")

	metadataPath := filepath.Join(repo, "pr-metadata.json")
	writeFile(t, metadataPath, `{}`)

	got, err := Collect(context.Background(), Options{
		Repo:         repo,
		ChangedFrom:  "HEAD~1",
		MetadataPath: metadataPath,
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	var found bool
	for _, item := range got {
		if item.Rule != "public_content_semantic_candidate" || item.File != "docs/public.md" {
			continue
		}
		found = true
		if !strings.Contains(item.Excerpt, "alpha-service rollout on Friday") {
			t.Fatalf("semantic candidate should include sanitized review text, got %#v", item)
		}
		if strings.Contains(item.Excerpt, "real-"+"secret-value") {
			t.Fatalf("semantic candidate leaked credential value: %#v", item)
		}
		if !strings.Contains(item.Excerpt, "SERVICE_TOKEN=<redacted>") {
			t.Fatalf("semantic candidate should redact credentials in review text, got %#v", item)
		}
		if !strings.Contains(item.Excerpt, "semantic signals") || !strings.Contains(item.Excerpt, "roadmap_timing") {
			t.Fatalf("semantic candidate excerpt should preserve semantic signals, got %#v", item)
		}
	}
	if !found {
		t.Fatalf("missing semantic candidate in findings %#v", got)
	}
}

func TestCollectSemanticCandidatesDoNotLeakWhitespaceCredentialTail(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "docs", "public.md"), "base\n")
	runGit(t, repo, "add", "docs/public.md")
	runGit(t, repo, "commit", "-m", "base")

	raw := "private launch plan for internal rollout on Friday with SERVICE_" + "TOKEN=\"real " + "secret value\""
	writeFile(t, filepath.Join(repo, "docs", "public.md"), "base\n"+raw+"\n")
	runGit(t, repo, "add", "docs/public.md")
	runGit(t, repo, "commit", "-m", "add semantic candidate")

	got := collectFromPreviousCommit(t, repo)
	for _, item := range got {
		if item.Rule != "public_content_semantic_candidate" || item.File != "docs/public.md" {
			continue
		}
		if strings.Contains(item.Excerpt, "secret value") || strings.Contains(item.Excerpt, "real "+"secret value") {
			t.Fatalf("semantic candidate leaked credential tail: %#v", item)
		}
		if !strings.Contains(item.Excerpt, "SERVICE_TOKEN=<redacted>") {
			t.Fatalf("semantic candidate should redact full credential assignment, got %#v", item)
		}
		return
	}
	t.Fatalf("missing semantic candidate in findings %#v", got)
}

func TestCollectJSONBearerHeadersDoNotLeakIntoSemanticCandidates(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "docs", "public.md"), "base\n")
	runGit(t, repo, "add", "docs/public.md")
	runGit(t, repo, "commit", "-m", "base")

	token := "abcdefghijklmnopqrstuvwxyz"
	raw := "private launch plan for internal rollout on Friday with " +
		`{"headers":{"Authorization":"Bearer ` + token + `"}}`
	writeFile(t, filepath.Join(repo, "docs", "public.md"), "base\n"+raw+"\n")
	runGit(t, repo, "add", "docs/public.md")
	runGit(t, repo, "commit", "-m", "add json bearer")

	got := collectFromPreviousCommit(t, repo)
	requireFinding(t, got, "docs/public.md", "public_content_bearer_header")
	for _, item := range got {
		if item.File != "docs/public.md" {
			continue
		}
		if strings.Contains(item.Excerpt, token) {
			t.Fatalf("finding leaked JSON bearer token: %#v", item)
		}
	}
}

func TestCollectDetectsQuotedJSONCredentialAssignments(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "docs", "public.json"), "{}\n")
	runGit(t, repo, "add", "docs/public.json")
	runGit(t, repo, "commit", "-m", "base")

	providerValue := "ghp_" + "1234567890abcdef1234567890abcdef1234"
	writeFile(t, filepath.Join(repo, "docs", "public.json"), strings.Join([]string{
		`{"access_` + `token":"` + providerValue + `"}`,
		`{"client_` + `secret": "` + providerValue + `"}`,
		`{"tenantAccess` + `Token":"` + providerValue + `"}`,
		`{"github` + `Token":"` + providerValue + `"}`,
		`{"vendorApi` + `Key":"` + providerValue + `"}`,
		`{"slackBot` + `Token":"xoxb_` + `1234567890abcdef"}`,
	}, "\n")+"\n")
	runGit(t, repo, "add", "docs/public.json")
	runGit(t, repo, "commit", "-m", "add json config")

	got := collectFromPreviousCommit(t, repo)
	var count int
	for _, item := range got {
		if item.File == "docs/public.json" && item.Rule == "public_content_generic_credential" {
			count++
			for _, forbidden := range []string{providerValue, "xoxb_" + "1234567890abcdef"} {
				if strings.Contains(item.Excerpt, forbidden) {
					t.Fatalf("JSON credential finding leaked value %q in excerpt %q", forbidden, item.Excerpt)
				}
			}
		}
	}
	if count != 6 {
		t.Fatalf("JSON credential findings = %d, want 6: %#v", count, got)
	}
}

func TestCollectAllowsBenignJSONTokenFields(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "docs", "public.json"), "{}\n")
	runGit(t, repo, "add", "docs/public.json")
	runGit(t, repo, "commit", "-m", "base")

	writeFile(t, filepath.Join(repo, "docs", "public.json"), strings.Join([]string{
		`{"tokenizer":"cl100k_base"}`,
		`{"token_count": 42}`,
		`{"page_token":"next"}`,
		`{"next_page_token":"next"}`,
		`{"file_token":"file-example"}`,
		`{"doc_token":"doc-example"}`,
		`{"node_token":"node-example"}`,
		`{"wiki_token":"wikcn_public_doc_example"}`,
		`{"folder_token":"folder-example"}`,
		`{"obj_token":"obj-example"}`,
		`{"spreadsheet_token":"sheet-example"}`,
		`{"parent_node_token":"parent-example"}`,
		`{"origin_node_token":"origin-example"}`,
		`{"drive_route_token":"route-example"}`,
		`{"token":"<wiki_token>"}`,
		`{"token":"wiki_token"}`,
		`{"token_url":"https://example.com/oauth/token"}`,
		`{"token_endpoint":"https://example.com/oauth/token"}`,
		`{"token_format":"Bearer"}`,
		`{"secret_name":"public-example-secret"}`,
		`{"base_token":"base-example"}`,
		`{"app_token":"app-example"}`,
		`{"sync_token":"sync-example"}`,
		`{"parent_token":"parent-example"}`,
		`{"target_token":"target-example"}`,
		`{"parent_file_token":"parent-file-example"}`,
		`{"refresh_token_expires_in": 7200}`,
		`{"access_token_expires_in": 7200}`,
		`{"token_expires_in": 7200}`,
		`{"token_status":"active"}`,
	}, "\n")+"\n")
	runGit(t, repo, "add", "docs/public.json")
	runGit(t, repo, "commit", "-m", "add benign json token fields")

	got := collectFromPreviousCommit(t, repo)
	for _, item := range got {
		if item.File == "docs/public.json" && item.Rule == "public_content_generic_credential" {
			t.Fatalf("benign JSON token field should not be credential finding: %#v", got)
		}
	}
}

func TestCollectDetectsAngleWrappedRealisticCredentialValues(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "docs", "config.yaml"), "base: true\n")
	runGit(t, repo, "add", "docs/config.yaml")
	runGit(t, repo, "commit", "-m", "base")
	stripeLike := "sk_" + "live_1234567890abcdef"
	patLike := "gh" + "p_1234567890abcdef1234567890abcdef1234"

	writeFile(t, filepath.Join(repo, "docs", "config.yaml"), strings.Join([]string{
		"API_KEY: <" + stripeLike + ">",
		"SECRET_TOKEN: <" + patLike + ">",
		"CLIENT_SECRET: <real-client-secret-value>",
	}, "\n")+"\n")
	runGit(t, repo, "add", "docs/config.yaml")
	runGit(t, repo, "commit", "-m", "add credential config")

	got := collectFromPreviousCommit(t, repo)
	var count int
	for _, item := range got {
		if item.File == "docs/config.yaml" && item.Rule == "public_content_generic_credential" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("angle-wrapped provider credential findings = %d, want 2: %#v", count, got)
	}
}

func TestCollectDetectsCredentialShapedValuesUnderBenignKeys(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "docs", "public.json"), "{}\n")
	runGit(t, repo, "add", "docs/public.json")
	runGit(t, repo, "commit", "-m", "base")
	stripeLike := "sk_" + "live_1234567890abcdef"
	patLike := "gh" + "p_1234567890abcdef1234567890abcdef1234"

	writeFile(t, filepath.Join(repo, "docs", "public.json"), strings.Join([]string{
		`{"access_token_expires_in":"` + patLike + `"}`,
		`{"refresh_token_expires_in":"` + stripeLike + `"}`,
		`{"client_secret_status":"real-client-secret-value"}`,
		`{"client_secret_name":"real-client-secret-value"}`,
		`{"app_token":"` + patLike + `"}`,
		`{"sync_token":"` + stripeLike + `"}`,
		`{"target_token":"real-client-secret-value"}`,
	}, "\n")+"\n")
	runGit(t, repo, "add", "docs/public.json")
	runGit(t, repo, "commit", "-m", "add credential-shaped benign fields")

	got := collectFromPreviousCommit(t, repo)
	var count int
	for _, item := range got {
		if item.File == "docs/public.json" && item.Rule == "public_content_generic_credential" {
			count++
		}
	}
	if count != 4 {
		t.Fatalf("provider-shaped benign-key findings = %d, want 4: %#v", count, got)
	}
}

func TestCollectAllowsBareIdentifierCredentialsWithMetadataSuffixes(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "docs", "config.yaml"), "base: true\n")
	runGit(t, repo, "add", "docs/config.yaml")
	runGit(t, repo, "commit", "-m", "base")

	writeFile(t, filepath.Join(repo, "docs", "config.yaml"), strings.Join([]string{
		"API_KEY_NAME: prod_key",
		"CLIENT_SECRET_NAME: prod_secret",
		"SECRET_STATUS: prod_secret",
	}, "\n")+"\n")
	runGit(t, repo, "add", "docs/config.yaml")
	runGit(t, repo, "commit", "-m", "add credential config")

	got := collectFromPreviousCommit(t, repo)
	for _, item := range got {
		if item.File == "docs/config.yaml" && item.Rule == "public_content_generic_credential" {
			t.Fatalf("readable metadata values should not be credential findings: %#v", got)
		}
	}
}

func TestCollectDetectsAccessKeyCredentials(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "docs", "config.yaml"), "base: true\n")
	runGit(t, repo, "add", "docs/config.yaml")
	runGit(t, repo, "commit", "-m", "base")
	accessKey := "AK" + "IAIOSFODNN7EXAMPXX"

	writeFile(t, filepath.Join(repo, "docs", "config.yaml"), strings.Join([]string{
		"AWS_ACCESS_KEY_ID: " + accessKey,
		"ACCESS_KEY_ID: " + accessKey,
		"ACCESS_KEY: " + accessKey,
	}, "\n")+"\n")
	runGit(t, repo, "add", "docs/config.yaml")
	runGit(t, repo, "commit", "-m", "add access key config")

	got := collectFromPreviousCommit(t, repo)
	var count int
	for _, item := range got {
		if item.File != "docs/config.yaml" || item.Rule != "public_content_generic_credential" {
			continue
		}
		count++
		if strings.Contains(item.Excerpt, accessKey) {
			t.Fatalf("access key finding leaked value in excerpt %q", item.Excerpt)
		}
	}
	if count != 3 {
		t.Fatalf("access key credential findings = %d, want 3: %#v", count, got)
	}
}

func TestCollectDetectsPrivateKeyAssignments(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "docs", "config.yaml"), "base: true\n")
	runGit(t, repo, "add", "docs/config.yaml")
	runGit(t, repo, "commit", "-m", "base")

	privateKey := "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0t"
	writeFile(t, filepath.Join(repo, "docs", "config.yaml"), strings.Join([]string{
		"PRIVATE_KEY: " + privateKey,
		"SSH_PRIVATE_KEY: " + privateKey,
		"JWT_PRIVATE_KEY: " + privateKey,
		"SIGNING_PRIVATE_KEY: " + privateKey,
	}, "\n")+"\n")
	runGit(t, repo, "add", "docs/config.yaml")
	runGit(t, repo, "commit", "-m", "add private key config")

	got := collectFromPreviousCommit(t, repo)
	var count int
	for _, item := range got {
		if item.File != "docs/config.yaml" || item.Rule != "public_content_generic_credential" {
			continue
		}
		count++
		if strings.Contains(item.Excerpt, privateKey) {
			t.Fatalf("private key finding leaked value in excerpt %q", item.Excerpt)
		}
	}
	if count != 4 {
		t.Fatalf("private key assignment findings = %d, want 4: %#v", count, got)
	}
}

func TestCollectAllowsCredentialValuesThatLookLikeBareIdentifiers(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "docs", "config.yaml"), "base: true\n")
	runGit(t, repo, "add", "docs/config.yaml")
	runGit(t, repo, "commit", "-m", "base")

	writeFile(t, filepath.Join(repo, "docs", "config.yaml"), strings.Join([]string{
		"API_KEY_OPENAI: prod_key",
		"CLIENT_SECRET_GOOGLE: prod_secret",
		"TOKEN_GITHUB: github_token",
		"APP_PASSWORD_PROD: prod_password",
	}, "\n")+"\n")
	runGit(t, repo, "add", "docs/config.yaml")
	runGit(t, repo, "commit", "-m", "add credential config")

	got := collectFromPreviousCommit(t, repo)
	for _, item := range got {
		if item.File == "docs/config.yaml" && item.Rule == "public_content_generic_credential" {
			t.Fatalf("readable identifiers should not be credential findings: %#v", got)
		}
	}
}

func TestCollectAllowsBenignUnquotedTokenFields(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "docs", "config.yaml"), "base: true\n")
	runGit(t, repo, "add", "docs/config.yaml")
	runGit(t, repo, "commit", "-m", "base")

	writeFile(t, filepath.Join(repo, "docs", "config.yaml"), strings.Join([]string{
		"tokens: 128",
		"token_type: bearer",
		"max_tokens: 2000",
		"completion_tokens: 200",
		"prompt_tokens: 100",
	}, "\n")+"\n")
	runGit(t, repo, "add", "docs/config.yaml")
	runGit(t, repo, "commit", "-m", "add benign token config")

	got := collectFromPreviousCommit(t, repo)
	for _, item := range got {
		if item.File == "docs/config.yaml" && item.Rule == "public_content_generic_credential" {
			t.Fatalf("benign unquoted token field should not be credential finding: %#v", got)
		}
	}
}

func TestCollectDetectsCredentialPhraseBeforeEnvironmentSuffix(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "docs", "config.yaml"), "base: true\n")
	runGit(t, repo, "add", "docs/config.yaml")
	runGit(t, repo, "commit", "-m", "base")

	providerValue := "ghp_" + "1234567890abcdef1234567890abcdef1234"
	writeFile(t, filepath.Join(repo, "docs", "config.yaml"), strings.Join([]string{
		"API_KEY_OPENAI: " + providerValue,
		"TOKEN_GITHUB: " + providerValue,
		"CLIENT_SECRET_GOOGLE: " + providerValue,
		"SECRET_KEY_BASE: " + providerValue,
		"APP_PASSWORD_PROD: " + providerValue,
	}, "\n")+"\n")
	runGit(t, repo, "add", "docs/config.yaml")
	runGit(t, repo, "commit", "-m", "add credential config")

	got := collectFromPreviousCommit(t, repo)
	var count int
	for _, item := range got {
		if item.File != "docs/config.yaml" || item.Rule != "public_content_generic_credential" {
			continue
		}
		count++
		for _, forbidden := range []string{providerValue} {
			if strings.Contains(item.Excerpt, forbidden) {
				t.Fatalf("credential finding leaked value %q in excerpt %q", forbidden, item.Excerpt)
			}
		}
	}
	if count != 5 {
		t.Fatalf("credential suffix variants findings = %d, want 5: %#v", count, got)
	}
}

func TestCollectDetectsPrivateKeyWhenOnlyEndIsAdded(t *testing.T) {
	repo := newGitRepo(t)

	writeFile(t, filepath.Join(repo, "docs", "key.pem"), privateKeyBegin()+"legacy-body\n")
	runGit(t, repo, "add", "docs/key.pem")
	runGit(t, repo, "commit", "-m", "base")

	writeFile(t, filepath.Join(repo, "docs", "key.pem"), privateKeyBegin()+"legacy-body\nnew-body\n"+privateKeyEnd())
	runGit(t, repo, "add", "docs/key.pem")
	runGit(t, repo, "commit", "-m", "complete key")

	got := collectFromPreviousCommit(t, repo)
	requireFinding(t, got, "docs/key.pem", "public_content_private_key_block")
}

func TestCollectDetectsPrivateKeyWhenOnlyBeginIsAdded(t *testing.T) {
	repo := newGitRepo(t)

	writeFile(t, filepath.Join(repo, "docs", "key.pem"), "legacy-body\n"+privateKeyEnd())
	runGit(t, repo, "add", "docs/key.pem")
	runGit(t, repo, "commit", "-m", "base")

	writeFile(t, filepath.Join(repo, "docs", "key.pem"), privateKeyBegin()+"legacy-body\n"+privateKeyEnd())
	runGit(t, repo, "add", "docs/key.pem")
	runGit(t, repo, "commit", "-m", "complete key")

	got := collectFromPreviousCommit(t, repo)
	requireFinding(t, got, "docs/key.pem", "public_content_private_key_block")
}

func TestCollectDetectsPrivateKeyWhenOnlyBodyIsAdded(t *testing.T) {
	repo := newGitRepo(t)

	writeFile(t, filepath.Join(repo, "docs", "key.pem"), privateKeyBegin()+privateKeyEnd())
	runGit(t, repo, "add", "docs/key.pem")
	runGit(t, repo, "commit", "-m", "base")

	writeFile(t, filepath.Join(repo, "docs", "key.pem"), privateKeyBegin()+"new-body\n"+privateKeyEnd())
	runGit(t, repo, "add", "docs/key.pem")
	runGit(t, repo, "commit", "-m", "add body")

	got := collectFromPreviousCommit(t, repo)
	requireFinding(t, got, "docs/key.pem", "public_content_private_key_block")
}

func TestCollectIgnoresUntouchedHistoricalPrivateKey(t *testing.T) {
	repo := newGitRepo(t)

	writeFile(t, filepath.Join(repo, "docs", "key.pem"), privateKeyBegin()+"legacy-body\n"+privateKeyEnd())
	runGit(t, repo, "add", "docs/key.pem")
	runGit(t, repo, "commit", "-m", "base")

	writeFile(t, filepath.Join(repo, "docs", "key.pem"), privateKeyBegin()+"legacy-body\n"+privateKeyEnd())
	writeFile(t, filepath.Join(repo, "docs", "public.md"), "public docs update\n")
	runGit(t, repo, "add", "docs/public.md")
	runGit(t, repo, "commit", "-m", "docs update")

	got := collectFromPreviousCommit(t, repo)
	for _, item := range got {
		if item.File == "docs/key.pem" && item.Rule == "public_content_private_key_block" {
			t.Fatalf("collector reported untouched historical private key: %#v", got)
		}
	}
}

func TestCollectIgnoresDeletedPrivateKeyLine(t *testing.T) {
	repo := newGitRepo(t)

	writeFile(t, filepath.Join(repo, "docs", "key.pem"), privateKeyBegin()+"legacy-body\n"+privateKeyEnd())
	runGit(t, repo, "add", "docs/key.pem")
	runGit(t, repo, "commit", "-m", "base")

	writeFile(t, filepath.Join(repo, "docs", "key.pem"), privateKeyBegin()+privateKeyEnd())
	runGit(t, repo, "add", "docs/key.pem")
	runGit(t, repo, "commit", "-m", "remove body")

	got := collectFromPreviousCommit(t, repo)
	for _, item := range got {
		if item.File == "docs/key.pem" && item.Rule == "public_content_private_key_block" {
			t.Fatalf("collector reported delete-only private key cleanup: %#v", got)
		}
	}
}

func TestCollectSkipsOnlyKnownQualityGateFixtureFiles(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")

	writeFile(t, filepath.Join(repo, "README.md"), "base\n")
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "base")

	writeFile(t, filepath.Join(repo, "internal", "qualitygate", "publiccontent", "collect_test.go"), "SECRET_TOKEN=fixture\n")
	writeFile(t, filepath.Join(repo, "internal", "qualitygate", "publiccontent", "scan_test.go"), "SECRET_TOKEN=fixture\n")
	writeFile(t, filepath.Join(repo, "internal", "qualitygate", "publiccontent", "scan.go"), "const privateKeyFixture = \""+privateKeyBeginPrefix+privateKeyMarker+"\"\n")
	writeFile(t, filepath.Join(repo, "internal", "qualitygate", "publiccontent", "rules.go"), "markers := []string{\"generated with automation\"}\n")
	providerValue := "ghp_" + "1234567890abcdef1234567890abcdef1234"
	writeFile(t, filepath.Join(repo, "tests", "e2e", "new-public-workflow.test.sh"), "SECRET_TOKEN="+providerValue+"\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "add scanner fixtures")

	metadataPath := filepath.Join(repo, "pr-metadata.json")
	writeFile(t, metadataPath, `{}`)

	got, err := Collect(context.Background(), Options{
		Repo:         repo,
		ChangedFrom:  "HEAD~1",
		MetadataPath: metadataPath,
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	var foundOrdinaryTestLeak bool
	for _, item := range got {
		switch item.File {
		case "internal/qualitygate/publiccontent/collect_test.go",
			"internal/qualitygate/publiccontent/scan.go",
			"internal/qualitygate/publiccontent/scan_test.go",
			"internal/qualitygate/publiccontent/rules.go":
			t.Fatalf("collector scanned known fixture or detector implementation file: %#v", got)
		}
		if item.File == "tests/e2e/new-public-workflow.test.sh" && item.Rule == "public_content_generic_credential" {
			foundOrdinaryTestLeak = true
		}
	}
	if !foundOrdinaryTestLeak {
		t.Fatalf("collector should still scan ordinary test files for real leaks: %#v", got)
	}
}

func TestScanChangedFileDocumentsFixtureExclusions(t *testing.T) {
	excluded := []string{
		"internal/qualitygate/publiccontent/collect_test.go",
		"internal/qualitygate/publiccontent/rules.go",
		"internal/qualitygate/publiccontent/scan.go",
		"internal/qualitygate/publiccontent/scan_test.go",
	}
	for _, file := range excluded {
		if scanChangedFile(file) {
			t.Fatalf("scanChangedFile(%q) = true, want false for detector fixture/implementation path", file)
		}
	}

	included := []string{
		"internal/qualitygate/publiccontent/new_test.go",
		"tests/e2e/new-public-workflow.test.sh",
		"docs/public.md",
	}
	for _, file := range included {
		if !scanChangedFile(file) {
			t.Fatalf("scanChangedFile(%q) = false, want true", file)
		}
	}
}

func TestCollectScansAddedLinesInSpecialPathNames(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "docs", "old.md"), "base\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "base")

	providerValue := "ghp_" + "1234567890abcdef1234567890abcdef1234"
	writeFile(t, filepath.Join(repo, "docs", "has space.md"), "SECRET_TOKEN="+providerValue+"\n")
	writeFile(t, filepath.Join(repo, `weird"quote.md`), "SECRET_TOKEN="+providerValue+"\n")
	runGit(t, repo, "mv", "docs/old.md", "docs/new name.md")
	writeFile(t, filepath.Join(repo, "docs", "new name.md"), "base\nSECRET_TOKEN="+providerValue+"\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "add special paths")

	got := collectFromPreviousCommit(t, repo)
	requireFinding(t, got, "docs/has space.md", "public_content_generic_credential")
	requireFinding(t, got, `weird"quote.md`, "public_content_generic_credential")
	requireFinding(t, got, "docs/new name.md", "public_content_generic_credential")
}

func TestCollectScansBranchNameAsWarning(t *testing.T) {
	repo := t.TempDir()
	metadataPath := filepath.Join(repo, "pr-metadata.json")
	writeFile(t, metadataPath, `{"branch":"bot/public-doc-update"}`)
	got, err := Collect(context.Background(), Options{
		Repo:         repo,
		MetadataPath: metadataPath,
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(got) != 1 || got[0].Rule != "public_content_automation_branch" {
		t.Fatalf("branch findings = %#v", got)
	}
}

func TestCollectUsesExplicitBranchNameWhenDetached(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "base\n")
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "base")
	runGit(t, repo, "checkout", "-b", "bot/public-doc-update")
	writeFile(t, filepath.Join(repo, "docs.md"), "safe docs\n")
	runGit(t, repo, "add", "docs.md")
	runGit(t, repo, "commit", "-m", "docs")
	head := strings.TrimSpace(string(runGitOutput(t, repo, "rev-parse", "HEAD")))
	runGit(t, repo, "checkout", "--detach", head)

	metadataPath := filepath.Join(repo, "pr-metadata.json")
	writeFile(t, metadataPath, `{}`)
	got, err := Collect(context.Background(), Options{
		Repo:         repo,
		MetadataPath: metadataPath,
		BranchName:   "bot/public-doc-update",
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	requireFinding(t, got, "branch", "public_content_automation_branch")
}

func TestCollectUsesBranchEnvironmentWhenDetached(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "base\n")
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "base")
	runGit(t, repo, "checkout", "-b", "bot/public-env-update")
	writeFile(t, filepath.Join(repo, "docs.md"), "safe docs\n")
	runGit(t, repo, "add", "docs.md")
	runGit(t, repo, "commit", "-m", "docs")
	head := strings.TrimSpace(string(runGitOutput(t, repo, "rev-parse", "HEAD")))
	runGit(t, repo, "checkout", "--detach", head)
	t.Setenv("GITHUB_HEAD_REF", "bot/public-env-update")

	metadataPath := filepath.Join(repo, "pr-metadata.json")
	writeFile(t, metadataPath, `{}`)
	got, err := Collect(context.Background(), Options{
		Repo:         repo,
		MetadataPath: metadataPath,
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	requireFinding(t, got, "branch", "public_content_automation_branch")
}

func TestCollectPreservesFindingAttributionForChangedLines(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, "docs", "auth.md"), "intro\n")
	runGit(t, repo, "add", "docs/auth.md")
	runGit(t, repo, "commit", "-m", "base")

	writeFile(t, filepath.Join(repo, "docs", "auth.md"), "intro\nAuthorization: Bearer abcdefghijklmnopqrstuvwxyz\n")
	runGit(t, repo, "add", "docs/auth.md")
	runGit(t, repo, "commit", "-m", "add auth docs")

	got := collectFromPreviousCommit(t, repo)
	for _, item := range got {
		if item.Rule == "public_content_bearer_header" {
			if item.File != "docs/auth.md" || item.Line != 2 || item.Source != "file" {
				t.Fatalf("changed-line attribution = %#v", item)
			}
			return
		}
	}
	t.Fatalf("missing bearer finding: %#v", got)
}

func TestAppendUniqueFindingsDeduplicatesByRuleFileLineAndSource(t *testing.T) {
	base := []Finding{newFinding("public_content_private_key_block", "docs/key.pem", 1, "file", "private key block")}
	got := appendUniqueFindings(base,
		newFinding("public_content_private_key_block", "docs/key.pem", 1, "file", "private key block"),
		newFinding("public_content_private_key_block", "docs/key.pem", 2, "file", "private key block"),
	)
	if len(got) != 2 {
		t.Fatalf("appendUniqueFindings len = %d, want 2: %#v", len(got), got)
	}
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	return repo
}

func privateKeyBegin() string {
	return privateKeyBeginPrefix + privateKeyMarker + "\n"
}

func privateKeyEnd() string {
	return privateKeyEndPrefix + privateKeyMarker + "\n"
}

func collectFromPreviousCommit(t *testing.T, repo string) []Finding {
	t.Helper()
	metadataPath := filepath.Join(repo, "pr-metadata.json")
	writeFile(t, metadataPath, `{}`)
	got, err := Collect(context.Background(), Options{
		Repo:         repo,
		ChangedFrom:  "HEAD~1",
		MetadataPath: metadataPath,
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	return got
}

func requireFinding(t *testing.T, got []Finding, file, rule string) {
	t.Helper()
	for _, item := range got {
		if item.File == file && item.Rule == rule {
			return
		}
	}
	t.Fatalf("missing %s in %s findings: %#v", rule, file, got)
}

func TestCollectRequiresValidMetadataJSON(t *testing.T) {
	repo := t.TempDir()
	metadataPath := filepath.Join(repo, "pr-metadata.json")
	writeFile(t, metadataPath, `{"title":`)

	_, err := Collect(context.Background(), Options{Repo: repo, MetadataPath: metadataPath})
	if err == nil || !strings.Contains(err.Error(), "public content metadata") {
		t.Fatalf("Collect() error = %v, want metadata parse error", err)
	}
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	if len(args) > 0 && args[0] == "commit" {
		args = append([]string{"commit", "--no-verify"}, args[1:]...)
	}
	cmd := gitcmd.Command(repo, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func runGitOutput(t *testing.T, repo string, args ...string) []byte {
	t.Helper()
	cmd := gitcmd.Command(repo, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return out
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
