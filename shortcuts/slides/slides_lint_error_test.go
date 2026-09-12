// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

// lintBlockMessage is the report the backend forwards when the layout lint
// refuses a write — the same document the lint script writes when it is run by
// hand. Written out as literal JSON rather than marshalled from the parser's own
// structs so a change to the server's field names has to be noticed here instead
// of being absorbed silently by both sides at once.
//
// It carries a warning alongside the two errors, because that is the report a
// real refusal produces and it is what proves the hint counts the errors rather
// than the findings. The second error carries a measurement and a
// related_objects pair, which this parser has no field for and which differ from
// rule to rule — they are here because the verbatim assertion below is what
// proves the CLI does not have to model a finding in order to deliver one.
const lintBlockMessage = `{"schema_version":"2.0","tool":"xml_lint","file":null,` +
	`"slide_size":{"width":960,"height":540},` +
	`"summary":{"slide_count":1,"error_count":2,"warning_count":1,"info_count":0,` +
	`"status":"blocked","release_ready":false,"screenshot_review_required":true},` +
	`"document":{"errors":[],"warnings":[],"infos":[]},` +
	`"slides":[{"slide_number":1,"status":"blocked","element_count":7,"errors":[` +
	`{"level":"error","code":"text_overflow","slide_number":1,"path":"presentation/slide/data/shape[2]",` +
	`"message":"text is 340pt tall in a 180pt shape","hint":"shorten the text or raise height"},` +
	`{"level":"error","code":"bbox_overlap","slide_number":1,` +
	`"message":"element ends at x=1080, slide is 960 wide",` +
	`"measurement":{"intersection_area":94.848,"width":9.88,"height":9.6},` +
	`"related_objects":[{"element_id":"bhT","xml_path":"slide[1]/data/shape[3]",` +
	`"bbox":{"x":36,"y":36,"width":9.88,"height":9.6}}]}],` +
	`"warnings":[{"level":"warning","code":"sparse_slide_content","slide_number":1,` +
	`"message":"coverage 12.8% below 15.0%"}],"infos":[]}]}`

// lintBlockedError builds the error the API layer produces for a refusal, which
// is a typed error carrying the engine's code and the report as its message.
func lintBlockedError(message string) error {
	return errs.NewAPIError(errs.SubtypeInvalidParameters, "%s", message).WithCode(lintBlockedCode)
}

// TestLintErrorKeepsTheReportVerbatimAndAddsTheEscapeHatch pins the division of
// labour: the finding is the server's to word and travels untouched, the flag is
// the CLI's to mention and travels in the hint.
//
// The message is asserted byte-for-byte because the same refusal also reaches
// callers through `lark-cli api`, where nothing rewrites it. Rendering it here
// would give one refusal two message formats depending on which command produced
// it, and a caller could not parse the field one way.
func TestLintErrorKeepsTheReportVerbatimAndAddsTheEscapeHatch(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_lint/slide",
		Body:   map[string]interface{}{"code": lintBlockedCode, "msg": lintBlockMessage},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_lint",
		"--slide", testPageXML,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected the lint block to surface as an error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %v, want typed problem metadata", err)
	}
	if problem.Category != errs.CategoryAPI {
		t.Fatalf("Category = %q, want %q: the backend rejection must survive enrichment",
			problem.Category, errs.CategoryAPI)
	}
	// Byte-for-byte, not "contains": every field of every finding reaches the
	// caller in the shape the server chose, so the same parse works whether the
	// refusal came through a shortcut or through `lark-cli api`.
	if problem.Message != lintBlockMessage {
		t.Fatalf("Message = %q, want the server document verbatim:\n%q", problem.Message, lintBlockMessage)
	}
	// The hint carries only what the document cannot say for itself.
	for _, want := range []string{
		"xml lint blocked: 2 error(s)",
		"--no-lint",
	} {
		if !strings.Contains(problem.Hint, want) {
			t.Fatalf("Hint lost %q\ngot: %q", want, problem.Hint)
		}
	}
	// The hint summarises, it does not restate: repeating the findings would put
	// two copies of the same list in one error, and the copies drift.
	if strings.Contains(problem.Hint, "text is 340pt tall") {
		t.Fatalf("Hint = %q, want a summary rather than a second copy of the findings", problem.Hint)
	}
}

// TestLintErrorCountsOnlyTheErrorsThatRefusedThePage covers a report spread
// across levels. Only errors refuse a write — the same line the lint script
// draws when it is run by hand — so a report that also carries a warning and an
// info is still a one-error refusal, and saying "3" would send the caller
// hunting for two more blockers that are not there.
func TestLintErrorCountsOnlyTheErrorsThatRefusedThePage(t *testing.T) {
	t.Parallel()

	msg := `{"schema_version":"2.0","tool":"xml_lint",` +
		`"summary":{"slide_count":3,"error_count":1,"warning_count":1,"info_count":1,` +
		`"status":"blocked","release_ready":false,"screenshot_review_required":true},` +
		`"schema_issues":"dropped unknown attribute shadow on shape[1]",` +
		`"slides":[{"slide_number":1,"status":"blocked","errors":[` +
		`{"level":"error","code":"out_of_bounds","message":"element ends at x=1080, slide is 960 wide"}],` +
		`"warnings":[{"level":"warning","code":"sparse_slide_content","message":"coverage 12.8% below 15.0%"}],` +
		`"infos":[{"level":"info","code":"table_resolved_size_mismatch",` +
		`"message":"table is 12pt narrower than declared"}]}]}`

	enriched := enrichSlidesLintError(lintBlockedError(msg))
	problem, ok := errs.ProblemOf(enriched)
	if !ok {
		t.Fatalf("err = %v, want typed problem metadata", enriched)
	}
	if problem.Message != msg {
		t.Fatalf("Message = %q, want the server document verbatim", problem.Message)
	}
	for _, want := range []string{
		// One error refused the page; the warning and the info came along.
		"xml lint blocked: 1 error(s)",
		// schema_issues travels in its own field, so a caller reading the message
		// top to bottom can miss it. The hint says it is there.
		"the message also carries schema_issues",
	} {
		if !strings.Contains(problem.Hint, want) {
			t.Fatalf("Hint lost %q\ngot: %q", want, problem.Hint)
		}
	}
	if strings.Contains(problem.Hint, "3 ") {
		t.Fatalf("Hint = %q, want the error count rather than the finding count", problem.Hint)
	}
}

// TestLintErrorLeavesOtherFailuresAlone guards the detection. The helper runs on
// every write path including ones the backend does not lint, so a false positive
// would staple "re-run with --no-lint" onto unrelated failures — advice that does
// nothing and sends the caller down the wrong path.
func TestLintErrorLeavesOtherFailuresAlone(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_other/slide",
		Body:   map[string]interface{}{"code": 3350001, "msg": "invalid slide xml"},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_other",
		"--slide", testPageXML,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected the backend rejection to surface")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %v, want typed problem metadata", err)
	}
	if strings.Contains(problem.Hint, "--no-lint") {
		t.Fatalf("Hint = %q, want no lint advice on a non-lint failure", problem.Hint)
	}
	// The 3350001 checklist still lands, so lint enrichment running first has not
	// displaced the existing fallback.
	if !strings.Contains(problem.Hint, "block_id not found in current slide") {
		t.Fatalf("Hint = %q, want the existing 3350001 checklist", problem.Hint)
	}
}

// TestLintErrorClaimsOnlyItsOwnCode is the other half of the detection. The
// backend reports other failures through the same field as JSON, and one of them
// could one day look like a lint report; the code is what says this refusal came
// from the lint gate, so a message alone must not be enough to claim it.
func TestLintErrorClaimsOnlyItsOwnCode(t *testing.T) {
	t.Parallel()

	for name, msg := range map[string]string{
		"a report under a foreign code": lintBlockMessage,
		"node server shape":             `{"code":"SCHEMA_INVALID","errors":[{"line":3,"detail":"unexpected element"}]}`,
		"not json":                      "invalid param",
	} {
		t.Run(name, func(t *testing.T) {
			original := errs.NewAPIError(errs.SubtypeInvalidParameters, "%s", msg).WithCode(4001000)
			enriched := enrichSlidesLintError(original)
			problem, ok := errs.ProblemOf(enriched)
			if !ok {
				t.Fatalf("err = %v, want typed problem metadata", enriched)
			}
			if problem.Message != msg {
				t.Fatalf("Message = %q, want it passed through unchanged", problem.Message)
			}
			if strings.Contains(problem.Hint, "--no-lint") {
				t.Fatalf("Hint = %q, want no lint advice on a message this helper does not own", problem.Hint)
			}
		})
	}
}

// TestLintErrorStillHelpsWhenTheReportCannotBeRead covers the code arriving with
// a message this parser cannot decode. The count is the half of the hint that
// needs the report; the escape hatch is the half the caller cannot get anywhere
// else, so dropping the whole hint over a missing number would withhold the only
// thing this layer knows.
func TestLintErrorStillHelpsWhenTheReportCannotBeRead(t *testing.T) {
	t.Parallel()

	enriched := enrichSlidesLintError(lintBlockedError("xml lint blocked"))
	problem, ok := errs.ProblemOf(enriched)
	if !ok {
		t.Fatalf("err = %v, want typed problem metadata", enriched)
	}
	if !strings.Contains(problem.Hint, "--no-lint") {
		t.Fatalf("Hint = %q, want the escape hatch even without a readable report", problem.Hint)
	}
	if strings.Contains(problem.Hint, "error(s)") {
		t.Fatalf("Hint = %q, want no invented count when the report did not parse", problem.Hint)
	}
}

// TestLintErrorSurvivesTheImageProgressHint covers the two enrichers landing on
// the same error. Both write to Hint, and a caller that uploaded images needs
// both facts: the pages were rejected, and the uploads already happened.
func TestLintErrorSurvivesTheImageProgressHint(t *testing.T) {
	t.Parallel()

	err := lintBlockedError(lintBlockMessage)
	enriched := appendSlidesProgressHint(enrichSlidesLintError(err), "2 image(s) were uploaded before the page failed")

	problem, ok := errs.ProblemOf(enriched)
	if !ok {
		t.Fatalf("err = %v, want typed problem metadata", enriched)
	}
	if !strings.Contains(problem.Hint, "--no-lint") {
		t.Fatalf("Hint = %q, want the lint escape hatch", problem.Hint)
	}
	if !strings.Contains(problem.Hint, "2 image(s) were uploaded") {
		t.Fatalf("Hint = %q, want the upload progress note", problem.Hint)
	}
}

// TestLintErrorScopesTheRefusalToThePage pins the wording against the case that
// exposed it. On +create the refusal can arrive after the deck and some of its
// pages are already on the server, and the orchestration hint says exactly that
// — so a lint hint claiming nothing was written puts two contradictory
// statements in the same message and the caller cannot tell which is true.
func TestLintErrorScopesTheRefusalToThePage(t *testing.T) {
	t.Parallel()

	err := lintBlockedError(lintBlockMessage)
	// The order +create produces: the lint enricher first, then the progress
	// note about what already landed.
	enriched := appendSlidesProgressHint(enrichSlidesLintError(err),
		"adding slide 2/3 failed; presentation pres_abc was created, 1 slide(s) added before failure")

	problem, ok := errs.ProblemOf(enriched)
	if !ok {
		t.Fatalf("err = %v, want typed problem metadata", enriched)
	}
	if strings.Contains(problem.Hint, "nothing was written") {
		t.Fatalf("Hint = %q, want no blanket claim that nothing was written: the same hint reports a created presentation and added slides", problem.Hint)
	}
	if !strings.Contains(problem.Hint, "the page was not written") {
		t.Fatalf("Hint = %q, want the refusal scoped to the page the gate rejected", problem.Hint)
	}
	if !strings.Contains(problem.Hint, "was created") {
		t.Fatalf("Hint = %q, want the orchestration progress preserved alongside it", problem.Hint)
	}
	// The findings in lintBlockMessage carry slide_number 1, because slide_number
	// counts inside the submission and every write path submits one page. Naming
	// it here would put "on slide 1" next to "adding slide 2/3 failed" and send
	// the caller to a page that linted clean. The page number has one source, and
	// it is the progress note.
	if strings.Contains(problem.Hint, "on slide") {
		t.Fatalf("Hint = %q, want no page named by the lint half: slide_number is the position inside the submitted page, not the deck", problem.Hint)
	}
}

// TestLintErrorPassesNilThrough keeps the helper safe to fold into a return
// expression on a path that may not have failed.
func TestLintErrorPassesNilThrough(t *testing.T) {
	t.Parallel()

	if got := enrichSlidesLintError(nil); got != nil {
		t.Fatalf("enrichSlidesLintError(nil) = %v, want nil", got)
	}
}
