// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package publiccontent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func Collect(ctx context.Context, opts Options) ([]Finding, error) {
	metadata, err := LoadMetadata(opts.MetadataPath)
	if err != nil {
		return nil, err
	}

	var out []Finding
	changedFiles, base, err := changedFiles(ctx, opts.Repo, opts.ChangedFrom)
	if err != nil {
		return nil, err
	}
	patches := map[string][]changedChunk{}
	if base != "" {
		patches, err = changedPatches(ctx, opts.Repo, base)
		if err != nil {
			return nil, err
		}
	}
	for _, file := range changedFiles {
		if !scanChangedFile(file) {
			continue
		}
		for _, chunk := range patches[file] {
			findings := scanText(file, "file", chunk.Text, isDetectorRuleFile(file))
			for i := range findings {
				findings[i].Line += chunk.StartLine - 1
			}
			out = append(out, findings...)
			out = append(out, semanticCandidate(file, "file", chunk.Text, chunk.StartLine)...)
		}
		privateKeyFindings, err := scanTouchedPrivateKeyBlocks(ctx, opts.Repo, file, patches[file])
		if err != nil {
			return nil, err
		}
		out = appendUniqueFindings(out, privateKeyFindings...)
	}
	if base != "" {
		commitFindings, err := scanCommitMessages(ctx, opts.Repo, base)
		if err != nil {
			return nil, err
		}
		out = append(out, commitFindings...)
	}
	branchName := opts.BranchName
	if branchName == "" {
		branchName = metadata.Branch
	}
	if branchName == "" {
		branchName = branchFromEnv()
	}
	if branchName == "" {
		branchName = currentBranch(ctx, opts.Repo)
	}
	if branchName != "" {
		out = append(out, scanText("branch", "branch", branchName, false)...)
	}
	out = append(out, scanMetadata(metadata)...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Rule < out[j].Rule
	})
	return out, nil
}

func currentBranch(ctx context.Context, repo string) string {
	data, err := gitOutput(ctx, repo, "branch", "--show-current")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func branchFromEnv() string {
	for _, key := range []string{"PR_BRANCH", "GITHUB_HEAD_REF", "GITHUB_REF_NAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func changedFiles(ctx context.Context, repo, changedFrom string) ([]string, string, error) {
	if changedFrom == "" {
		return nil, "", nil
	}
	baseBytes, err := gitOutput(ctx, repo, "merge-base", changedFrom, "HEAD")
	if err != nil {
		return nil, "", err
	}
	base := strings.TrimSpace(string(baseBytes))
	files, err := diffFileNames(ctx, repo, base)
	if err != nil {
		return nil, "", err
	}
	sort.Strings(files)
	return files, base, nil
}

func diffFileNames(ctx context.Context, repo, base string) ([]string, error) {
	data, err := gitOutput(ctx, repo, "diff", "--name-only", "-z", "--diff-filter=ACMR", base+"..HEAD")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, file := range bytes.Split(data, []byte{0}) {
		if len(file) == 0 {
			continue
		}
		files = append(files, filepath.ToSlash(string(file)))
	}
	return files, nil
}

var detectorFixtureExclusions = map[string]bool{
	"internal/qualitygate/publiccontent/collect_test.go": true,
	"internal/qualitygate/publiccontent/rules.go":        true,
	"internal/qualitygate/publiccontent/scan.go":         true,
	"internal/qualitygate/publiccontent/scan_test.go":    true,
}

func scanChangedFile(file string) bool {
	normalized := strings.TrimPrefix(strings.ReplaceAll(file, "\\", "/"), "./")
	return !detectorFixtureExclusions[normalized]
}

type changedChunk struct {
	StartLine int
	Text      string
}

func (c changedChunk) endLine() int {
	lines := strings.Count(strings.TrimRight(c.Text, "\n"), "\n") + 1
	if lines < 1 {
		lines = 1
	}
	return c.StartLine + lines - 1
}

func changedPatches(ctx context.Context, repo, base string) (map[string][]changedChunk, error) {
	files, err := diffFileNames(ctx, repo, base)
	if err != nil {
		return nil, err
	}
	data, err := gitOutput(ctx, repo, "diff", "--no-ext-diff", "--unified=0", "--diff-filter=ACMR", base+"..HEAD")
	if err != nil {
		return nil, err
	}
	out := map[string][]changedChunk{}
	var file string
	var chunk *changedChunk
	nextLine := 0
	nextFile := 0
	flush := func() {
		if file == "" || chunk == nil || chunk.Text == "" {
			chunk = nil
			return
		}
		out[file] = append(out[file], *chunk)
		chunk = nil
	}
	for _, raw := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(raw, "diff --git "):
			flush()
			file = ""
			if nextFile < len(files) {
				file = files[nextFile]
				nextFile++
			}
		case strings.HasPrefix(raw, "@@ "):
			flush()
			start, ok := parseNewHunkStart(raw)
			if !ok {
				nextLine = 0
				continue
			}
			nextLine = start
			chunk = &changedChunk{StartLine: start}
		case strings.HasPrefix(raw, "+") && !strings.HasPrefix(raw, "+++"):
			if chunk == nil {
				chunk = &changedChunk{StartLine: max(nextLine, 1)}
			}
			chunk.Text += strings.TrimPrefix(raw, "+") + "\n"
			nextLine++
		case strings.HasPrefix(raw, "-"):
			continue
		default:
			if chunk != nil && strings.HasPrefix(raw, `\ No newline at end of file`) {
				continue
			}
			flush()
		}
	}
	flush()
	return out, nil
}

func parseNewHunkStart(header string) (int, bool) {
	parts := strings.Split(header, " ")
	for _, part := range parts {
		if !strings.HasPrefix(part, "+") {
			continue
		}
		raw := strings.TrimPrefix(part, "+")
		if before, _, ok := strings.Cut(raw, ","); ok {
			raw = before
		}
		start, err := strconv.Atoi(raw)
		return start, err == nil && start > 0
	}
	return 0, false
}

func scanCommitMessages(ctx context.Context, repo, base string) ([]Finding, error) {
	data, err := gitOutput(ctx, repo, "log", "--format=%H%x00%B%x00", base+"..HEAD")
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(data, []byte{0})
	var out []Finding
	for i := 0; i+1 < len(parts); i += 2 {
		sha := strings.TrimSpace(string(parts[i]))
		body := string(parts[i+1])
		if sha == "" || body == "" {
			continue
		}
		short := sha
		if len(short) > 12 {
			short = short[:12]
		}
		out = append(out, scanText("commit:"+short, "commit", body, false)...)
		out = append(out, semanticCandidate("commit:"+short, "commit", body, 1)...)
	}
	return out, nil
}

type lineRange struct {
	Start int
	End   int
}

func scanTouchedPrivateKeyBlocks(ctx context.Context, repo, file string, chunks []changedChunk) ([]Finding, error) {
	if len(chunks) == 0 {
		return nil, nil
	}
	data, err := gitOutput(ctx, repo, "show", "HEAD:"+file)
	if err != nil {
		return nil, err
	}
	var added []lineRange
	for _, chunk := range chunks {
		added = append(added, lineRange{Start: chunk.StartLine, End: chunk.endLine()})
	}
	var out []Finding
	for _, block := range privateKeyBlocks(string(data)) {
		if !rangesIntersectAny(block, added) {
			continue
		}
		out = append(out, newFinding("public_content_private_key_block", file, block.Start, "file", "private key block"))
	}
	return out, nil
}

func privateKeyBlocks(text string) []lineRange {
	lines := strings.Split(text, "\n")
	var out []lineRange
	inPrivateKey := false
	start := 0
	for i, line := range lines {
		lineNo := i + 1
		if !inPrivateKey && strings.Contains(line, privateKeyBeginPrefix) && strings.Contains(line, privateKeyMarker) {
			inPrivateKey = true
			start = lineNo
		}
		if inPrivateKey && strings.Contains(line, privateKeyEndPrefix) && strings.Contains(line, privateKeyMarker) {
			out = append(out, lineRange{Start: start, End: lineNo})
			inPrivateKey = false
		}
	}
	return out
}

func rangesIntersectAny(block lineRange, ranges []lineRange) bool {
	for _, r := range ranges {
		if block.Start <= r.End && r.Start <= block.End {
			return true
		}
	}
	return false
}

func appendUniqueFindings(items []Finding, additions ...Finding) []Finding {
	for _, addition := range additions {
		duplicate := false
		for _, item := range items {
			if item.Rule == addition.Rule &&
				item.File == addition.File &&
				item.Line == addition.Line &&
				item.Source == addition.Source {
				duplicate = true
				break
			}
		}
		if !duplicate {
			items = append(items, addition)
		}
	}
	return items
}

func gitOutput(ctx context.Context, repo string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repo
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, stderr.Bytes())
	}
	return stdout.Bytes(), nil
}
