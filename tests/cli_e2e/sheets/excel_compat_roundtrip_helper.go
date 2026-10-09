// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"archive/zip"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type excelCompatRoundTripOptions struct {
	CLI               string
	Source            string
	OutputDir         string
	Profile           string
	As                string
	ScreenshotCommand []string
	RenderCommand     []string
	Environment       map[string]string
	CLIEnvironment    map[string]string
}

type excelCompatRoundTripResult struct {
	ImportedToken    string `json:"imported_token"`
	ExportedFile     string `json:"exported_file"`
	ReimportedToken  string `json:"reimported_token"`
	importedTicket   string
	reimportedTicket string
}

type excelCompatEnvelope struct {
	OK   *bool `json:"ok"`
	Data struct {
		Ready     bool   `json:"ready"`
		Failed    bool   `json:"failed"`
		Token     string `json:"token"`
		Ticket    string `json:"ticket"`
		SavedPath string `json:"saved_path"`
		StatusMsg string `json:"status_msg"`
		FileToken string `json:"file_token"`
		FileName  string `json:"file_name"`
		Deleted   bool   `json:"deleted"`
		TaskID    string `json:"task_id"`
		Metas     []struct {
			DocToken string `json:"doc_token"`
			DocType  string `json:"doc_type"`
			URL      string `json:"url"`
		} `json:"metas"`
	} `json:"data"`
}

type excelCompatStageManifest struct {
	Stage      string `json:"stage"`
	Token      string `json:"token"`
	Workbook   string `json:"workbook"`
	Provenance struct {
		Kind string `json:"kind"`
	} `json:"provenance"`
	Screenshots []struct {
		SheetIndex int    `json:"sheet_index"`
		SheetName  string `json:"sheet_name"`
		Stage      string `json:"stage"`
		File       string `json:"file"`
	} `json:"screenshots"`
}

type excelCompatImportResource struct {
	token  string
	ticket string
}

type excelCompatSheetIdentity struct {
	Index int
	Name  string
}

const (
	excelCompatSpreadsheetTransitionalNS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	excelCompatSpreadsheetStrictNS       = "http://purl.oclc.org/ooxml/spreadsheetml/main"
	excelCompatRelationshipsTransitional = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	excelCompatRelationshipsStrict       = "http://purl.oclc.org/ooxml/officeDocument/relationships"
	excelCompatWorksheetContentType      = "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
	excelCompatMaxPNGDimension           = 8192
	excelCompatMaxPNGPixelCount          = 16_000_000
)

type excelCompatWorkbookXML struct {
	XMLName xml.Name
	Sheets  []excelCompatWorkbookSheet `xml:"sheets>sheet"`
}

type excelCompatWorkbookSheet struct {
	Name string
	ID   string
}

func (sheet *excelCompatWorkbookSheet) UnmarshalXML(
	decoder *xml.Decoder,
	start xml.StartElement,
) error {
	for _, attribute := range start.Attr {
		switch attribute.Name.Local {
		case "name":
			sheet.Name = attribute.Value
		case "id":
			if attribute.Name.Space == excelCompatRelationshipsTransitional ||
				attribute.Name.Space == excelCompatRelationshipsStrict {
				sheet.ID = attribute.Value
			}
		}
	}
	return decoder.Skip()
}

func runExcelCompatRoundTrip(ctx context.Context, opts excelCompatRoundTripOptions) (_ excelCompatRoundTripResult, returnErr error) {
	var result excelCompatRoundTripResult
	if len(opts.ScreenshotCommand) == 0 {
		return result, errors.New("screenshot command is required for imported and reimported stages")
	}
	if len(opts.RenderCommand) == 0 {
		return result, errors.New("render command is required for source and exported stages")
	}
	if strings.TrimSpace(opts.CLI) == "" || !filepath.IsAbs(opts.CLI) {
		return result, errors.New("lark-cli path must be absolute")
	}
	if strings.TrimSpace(opts.Source) == "" || !filepath.IsAbs(opts.Source) {
		return result, errors.New("source workbook path must be absolute")
	}
	sourceSheets, err := excelCompatWorkbookSheets(opts.Source)
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(opts.OutputDir) == "" || !filepath.IsAbs(opts.OutputDir) {
		return result, errors.New("output directory must be absolute")
	}
	if opts.As == "" {
		opts.As = "bot"
	}
	if err := os.MkdirAll(opts.OutputDir, 0o700); err != nil {
		return result, fmt.Errorf("create output directory: %w", err)
	}
	if err := resetExcelCompatOwnedArtifacts(opts.OutputDir); err != nil {
		return result, err
	}
	if err := runExcelCompatRender(ctx, opts, opts.Source, "source"); err != nil {
		return result, err
	}

	defer func() {
		returnErr = errors.Join(returnErr, cleanupExcelCompatResources(ctx, opts, []excelCompatImportResource{
			{token: result.ImportedToken, ticket: result.importedTicket},
			{token: result.ReimportedToken, ticket: result.reimportedTicket},
		}))
	}()

	imported, err := runExcelCompatCLIAt(
		ctx, opts, filepath.Dir(opts.Source),
		"drive", "+import", "--file", filepath.Base(opts.Source), "--type", "sheet", "--no-wait",
	)
	if err != nil {
		return result, fmt.Errorf("import source workbook: %w", err)
	}
	result.importedTicket = imported.Data.Ticket
	result.ImportedToken, err = waitExcelCompatImport(ctx, opts, imported)
	if err != nil {
		return result, fmt.Errorf("wait for source workbook import: %w", err)
	}
	if err := runExcelCompatScreenshot(ctx, opts, result.ImportedToken, "imported", sourceSheets); err != nil {
		return result, err
	}

	exportedName := "exported.xlsx"
	exported, err := runExcelCompatCLI(
		ctx, opts, "drive", "+export",
		"--token", result.ImportedToken,
		"--doc-type", "sheet",
		"--file-extension", "xlsx",
		"--file-name", exportedName,
		"--output-dir", opts.OutputDir,
		"--overwrite",
	)
	if err != nil {
		return result, fmt.Errorf("export imported workbook: %w", err)
	}
	result.ExportedFile, err = waitExcelCompatExport(
		ctx, opts, result.ImportedToken, exportedName, exported,
	)
	if err != nil {
		return result, fmt.Errorf("wait for imported workbook export: %w", err)
	}
	if !filepath.IsAbs(result.ExportedFile) {
		result.ExportedFile = filepath.Join(opts.OutputDir, result.ExportedFile)
	}
	if _, err := os.Stat(result.ExportedFile); err != nil {
		return result, fmt.Errorf("exported workbook is unavailable: %w", err)
	}
	exportedSheets, err := excelCompatWorkbookSheets(result.ExportedFile)
	if err != nil {
		return result, fmt.Errorf("validate exported workbook: %w", err)
	}
	if err := runExcelCompatRender(ctx, opts, result.ExportedFile, "exported"); err != nil {
		return result, err
	}

	reimported, err := runExcelCompatCLIAt(
		ctx, opts, filepath.Dir(result.ExportedFile),
		"drive", "+import", "--file", filepath.Base(result.ExportedFile), "--type", "sheet", "--no-wait",
	)
	if err != nil {
		return result, fmt.Errorf("reimport exported workbook: %w", err)
	}
	result.reimportedTicket = reimported.Data.Ticket
	result.ReimportedToken, err = waitExcelCompatImport(ctx, opts, reimported)
	if err != nil {
		return result, fmt.Errorf("wait for exported workbook reimport: %w", err)
	}
	if err := runExcelCompatScreenshot(ctx, opts, result.ReimportedToken, "reimported", exportedSheets); err != nil {
		return result, err
	}
	if err := validateExcelCompatStageSet(
		opts.OutputDir,
		opts.Source,
		result.ImportedToken,
		result.ExportedFile,
		result.ReimportedToken,
	); err != nil {
		return result, fmt.Errorf("validate four-stage screenshot evidence: %w", err)
	}

	return result, nil
}

func validateExcelCompatSource(path string) error {
	_, err := excelCompatWorkbookSheets(path)
	return err
}

func excelCompatWorkbookSheets(path string) ([]excelCompatSheetIdentity, error) {
	if strings.ToLower(filepath.Ext(path)) != ".xlsx" {
		return nil, errors.New("source workbook must use the .xlsx extension")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat source workbook: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("source workbook must be a regular file")
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("source workbook is not a valid XLSX container: %w", err)
	}
	defer reader.Close()

	files := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		files[file.Name] = file
	}
	for _, required := range []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"xl/workbook.xml",
		"xl/_rels/workbook.xml.rels",
	} {
		if files[required] == nil {
			return nil, fmt.Errorf("source workbook is missing %s", required)
		}
	}

	var workbook excelCompatWorkbookXML
	if err := decodeExcelCompatXML(files["xl/workbook.xml"], &workbook); err != nil {
		return nil, fmt.Errorf("decode xl/workbook.xml: %w", err)
	}
	if workbook.XMLName.Local != "workbook" ||
		(workbook.XMLName.Space != excelCompatSpreadsheetTransitionalNS &&
			workbook.XMLName.Space != excelCompatSpreadsheetStrictNS) {
		return nil, fmt.Errorf("source workbook has unsupported workbook namespace %q", workbook.XMLName.Space)
	}
	if len(workbook.Sheets) == 0 {
		return nil, errors.New("source workbook contains no sheets")
	}

	var relationships struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
			Type   string `xml:"Type,attr"`
		} `xml:"Relationship"`
	}
	if err := decodeExcelCompatXML(files["xl/_rels/workbook.xml.rels"], &relationships); err != nil {
		return nil, fmt.Errorf("decode workbook relationships: %w", err)
	}
	type relationshipTarget struct {
		target string
		kind   string
	}
	targetByID := make(map[string]relationshipTarget, len(relationships.Items))
	for _, relationship := range relationships.Items {
		targetByID[relationship.ID] = relationshipTarget{
			target: relationship.Target,
			kind:   relationship.Type,
		}
	}

	var contentTypes struct {
		Overrides []struct {
			PartName    string `xml:"PartName,attr"`
			ContentType string `xml:"ContentType,attr"`
		} `xml:"Override"`
	}
	if err := decodeExcelCompatXML(files["[Content_Types].xml"], &contentTypes); err != nil {
		return nil, fmt.Errorf("decode content types: %w", err)
	}
	contentTypeByPart := make(map[string]string, len(contentTypes.Overrides))
	for _, override := range contentTypes.Overrides {
		contentTypeByPart[pathpkg.Clean(strings.TrimPrefix(override.PartName, "/"))] = override.ContentType
	}

	result := make([]excelCompatSheetIdentity, 0, len(workbook.Sheets))
	seenNames := make(map[string]struct{}, len(workbook.Sheets))
	for index, sheet := range workbook.Sheets {
		if strings.TrimSpace(sheet.Name) == "" || sheet.ID == "" {
			return nil, errors.New("source workbook has an incomplete sheet definition")
		}
		if _, exists := seenNames[sheet.Name]; exists {
			return nil, fmt.Errorf("source workbook has duplicate sheet name %q", sheet.Name)
		}
		seenNames[sheet.Name] = struct{}{}
		relationship := targetByID[sheet.ID]
		rawTarget := strings.TrimSpace(relationship.target)
		target := ""
		if strings.HasPrefix(rawTarget, "/") {
			target = pathpkg.Clean(strings.TrimPrefix(rawTarget, "/"))
		} else {
			target = pathpkg.Clean(pathpkg.Join("xl", rawTarget))
		}
		if rawTarget == "" ||
			(target != "xl" && !strings.HasPrefix(target, "xl/")) ||
			files[target] == nil {
			return nil, fmt.Errorf("source workbook sheet %q has an invalid relationship", sheet.Name)
		}
		if relationship.kind != excelCompatRelationshipsTransitional+"/worksheet" &&
			relationship.kind != excelCompatRelationshipsStrict+"/worksheet" {
			return nil, fmt.Errorf("source workbook sheet %q relationship is not a worksheet", sheet.Name)
		}
		if contentTypeByPart[target] != excelCompatWorksheetContentType {
			return nil, fmt.Errorf("source workbook sheet %q has an invalid content type", sheet.Name)
		}
		rootName, err := decodeExcelCompatRootName(files[target])
		if err != nil {
			return nil, fmt.Errorf("decode worksheet %q: %w", sheet.Name, err)
		}
		if rootName.Local != "worksheet" ||
			(rootName.Space != excelCompatSpreadsheetTransitionalNS &&
				rootName.Space != excelCompatSpreadsheetStrictNS) {
			return nil, fmt.Errorf("source workbook sheet %q target is not a worksheet", sheet.Name)
		}
		result = append(result, excelCompatSheetIdentity{Index: index, Name: sheet.Name})
	}
	return result, nil
}

func decodeExcelCompatXML(file *zip.File, destination interface{}) error {
	stream, err := file.Open()
	if err != nil {
		return err
	}
	defer stream.Close()
	decoder := xml.NewDecoder(io.LimitReader(stream, 4<<20))
	return decoder.Decode(destination)
}

func decodeExcelCompatRootName(file *zip.File) (xml.Name, error) {
	stream, err := file.Open()
	if err != nil {
		return xml.Name{}, err
	}
	defer stream.Close()
	decoder := xml.NewDecoder(io.LimitReader(stream, 4<<20))
	for {
		token, err := decoder.Token()
		if err != nil {
			return xml.Name{}, err
		}
		if start, ok := token.(xml.StartElement); ok {
			return start.Name, nil
		}
	}
}

func cleanupExcelCompatResources(
	parent context.Context,
	opts excelCompatRoundTripOptions,
	resources []excelCompatImportResource,
) error {
	var cleanupErrs []error
	for _, resource := range resources {
		if resource.token == "" && resource.ticket == "" {
			continue
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(parent), 2*time.Minute)
		token := resource.token
		if token == "" {
			var err error
			token, err = recoverExcelCompatImportToken(cleanupCtx, opts, resource.ticket)
			if err != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf(
					"recover import token for ticket %s: %w", resource.ticket, err,
				))
				cleanupCancel()
				continue
			}
		}
		if token == "" {
			cleanupCancel()
			continue
		}
		if err := deleteExcelCompatSheet(cleanupCtx, opts, token); err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
		cleanupCancel()
	}
	return errors.Join(cleanupErrs...)
}

func recoverExcelCompatImportToken(
	ctx context.Context,
	opts excelCompatRoundTripOptions,
	ticket string,
) (string, error) {
	for {
		envelope, err := runExcelCompatCLI(
			ctx, opts, "drive", "+task_result",
			"--scenario", "import",
			"--ticket", ticket,
		)
		if err == nil {
			if envelope.Data.Failed {
				return "", nil
			}
			if envelope.Data.Ready {
				if envelope.Data.Token == "" {
					return "", errors.New("ready cleanup import task returned no token")
				}
				return envelope.Data.Token, nil
			}
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			if err != nil {
				return "", errors.Join(ctx.Err(), err)
			}
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

func deleteExcelCompatSheet(
	ctx context.Context,
	opts excelCompatRoundTripOptions,
	token string,
) error {
	deleteEnvelope, deleteErr := runExcelCompatCLI(
		ctx, opts, "drive", "+delete",
		"--file-token", token,
		"--type", "sheet",
		"--yes",
	)
	if deleteErr == nil &&
		!deleteEnvelope.Data.Deleted &&
		deleteEnvelope.Data.TaskID == "" &&
		!deleteEnvelope.Data.Ready {
		deleteErr = errors.New("delete returned no completion evidence")
	}
	if deleteErr == nil && deleteEnvelope.Data.TaskID != "" {
		deleteErr = waitExcelCompatDeleteTask(ctx, opts, deleteEnvelope.Data.TaskID)
	}

	for {
		deleted, verifyErr := isExcelCompatSheetDeleted(ctx, opts, token)
		if verifyErr == nil && deleted {
			return nil
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf(
				"delete sheet %s was not verified: %w",
				token,
				errors.Join(deleteErr, verifyErr, ctx.Err()),
			)
		case <-timer.C:
		}
	}
}

func waitExcelCompatDeleteTask(
	ctx context.Context,
	opts excelCompatRoundTripOptions,
	taskID string,
) error {
	for {
		envelope, err := runExcelCompatCLI(
			ctx, opts, "drive", "+task_result",
			"--scenario", "task_check",
			"--task-id", taskID,
		)
		if err != nil {
			return fmt.Errorf("query delete task %s: %w", taskID, err)
		}
		if envelope.Data.Failed {
			return fmt.Errorf("delete task %s failed", taskID)
		}
		if envelope.Data.Ready {
			return nil
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func isExcelCompatSheetDeleted(
	ctx context.Context,
	opts excelCompatRoundTripOptions,
	token string,
) (bool, error) {
	data, err := json.Marshal(map[string]interface{}{
		"request_docs": []map[string]string{{
			"doc_token": token,
			"doc_type":  "sheet",
		}},
		"with_url": true,
	})
	if err != nil {
		return false, err
	}
	envelope, err := runExcelCompatCLI(
		ctx, opts, "api", "post",
		"/open-apis/drive/v1/metas/batch_query",
		"--data", string(data),
	)
	if err != nil {
		return false, err
	}
	if len(envelope.Data.Metas) == 0 {
		return true, nil
	}
	for _, meta := range envelope.Data.Metas {
		if meta.DocToken != token || meta.DocType != "sheet" {
			return false, fmt.Errorf(
				"metadata identity mismatch: got %s/%s, want sheet/%s",
				meta.DocType, meta.DocToken, token,
			)
		}
		if strings.TrimSpace(meta.URL) == "" {
			return false, errors.New("metadata for existing sheet omitted URL despite with_url=true")
		}
		return false, nil
	}
	return false, errors.New("metadata response was ambiguous")
}

func waitExcelCompatExport(
	ctx context.Context,
	opts excelCompatRoundTripOptions,
	sourceToken string,
	exportedName string,
	envelope excelCompatEnvelope,
) (string, error) {
	ticket := envelope.Data.Ticket
	for {
		if envelope.Data.SavedPath != "" {
			return envelope.Data.SavedPath, nil
		}
		if envelope.Data.Failed {
			if envelope.Data.StatusMsg != "" {
				return "", fmt.Errorf("export task failed: %s", envelope.Data.StatusMsg)
			}
			return "", errors.New("export task failed")
		}
		if envelope.Data.Ready {
			if envelope.Data.FileToken == "" {
				return "", errors.New("ready export task returned no file token")
			}
			downloaded, err := runExcelCompatCLI(
				ctx, opts, "drive", "+export-download",
				"--file-token", envelope.Data.FileToken,
				"--file-name", exportedName,
				"--output-dir", opts.OutputDir,
				"--overwrite",
			)
			if err != nil {
				return "", fmt.Errorf("download export artifact: %w", err)
			}
			if downloaded.Data.SavedPath == "" {
				return "", errors.New("export download returned no saved path")
			}
			return downloaded.Data.SavedPath, nil
		}
		if ticket == "" {
			return "", errors.New("pending export task returned no ticket")
		}

		var err error
		envelope, err = runExcelCompatCLI(
			ctx, opts, "drive", "+task_result",
			"--scenario", "export",
			"--ticket", ticket,
			"--file-token", sourceToken,
		)
		if err != nil {
			return "", fmt.Errorf("query export task %s: %w", ticket, err)
		}
		if envelope.Data.Ready || envelope.Data.Failed {
			continue
		}

		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

func waitExcelCompatImport(
	ctx context.Context,
	opts excelCompatRoundTripOptions,
	envelope excelCompatEnvelope,
) (string, error) {
	ticket := envelope.Data.Ticket
	for {
		if envelope.Data.Failed {
			if envelope.Data.StatusMsg != "" {
				return "", fmt.Errorf("import task failed: %s", envelope.Data.StatusMsg)
			}
			return "", errors.New("import task failed")
		}
		if envelope.Data.Ready {
			if envelope.Data.Token == "" {
				return "", errors.New("ready import task returned no token")
			}
			return envelope.Data.Token, nil
		}
		if ticket == "" {
			return "", errors.New("pending import task returned no ticket")
		}

		var err error
		envelope, err = runExcelCompatCLI(
			ctx, opts, "drive", "+task_result",
			"--scenario", "import",
			"--ticket", ticket,
		)
		if err != nil {
			return "", fmt.Errorf("query import task %s: %w", ticket, err)
		}
		if envelope.Data.Ready || envelope.Data.Failed {
			continue
		}

		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

func runExcelCompatCLI(ctx context.Context, opts excelCompatRoundTripOptions, args ...string) (excelCompatEnvelope, error) {
	return runExcelCompatCLIAt(ctx, opts, "", args...)
}

func runExcelCompatCLIAt(
	ctx context.Context,
	opts excelCompatRoundTripOptions,
	workDir string,
	args ...string,
) (excelCompatEnvelope, error) {
	var envelope excelCompatEnvelope
	cliArgs := make([]string, 0, len(args)+4)
	if opts.Profile != "" {
		cliArgs = append(cliArgs, "--profile", opts.Profile)
	}
	cliArgs = append(cliArgs, args...)
	if opts.As != "" {
		cliArgs = append(cliArgs, "--as", opts.As)
	}
	cliArgs = append(cliArgs, "--json")

	command := exec.CommandContext(ctx, opts.CLI, cliArgs...)
	if workDir != "" {
		command.Dir = workDir
	}
	commandEnv, err := excelCompatCLICommandEnv(
		opts.Environment, opts.CLIEnvironment,
	)
	if err != nil {
		return envelope, err
	}
	command.Env = commandEnv
	var stdout strings.Builder
	var stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	if err != nil {
		return envelope, fmt.Errorf("%s: %w", strings.TrimSpace(stderr.String()), err)
	}
	if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil {
		return envelope, fmt.Errorf("decode lark-cli response: %w", err)
	}
	if envelope.OK == nil || !*envelope.OK {
		return envelope, errors.New("lark-cli response must contain ok=true")
	}
	return envelope, nil
}

func runExcelCompatScreenshot(
	ctx context.Context,
	opts excelCompatRoundTripOptions,
	token string,
	stage string,
	expectedSheets []excelCompatSheetIdentity,
) error {
	if err := resetExcelCompatStageDir(opts.OutputDir, stage); err != nil {
		return err
	}
	args := make([]string, 0, len(opts.ScreenshotCommand)-1)
	for _, value := range opts.ScreenshotCommand[1:] {
		replacer := strings.NewReplacer(
			"{token}", token,
			"{stage}", stage,
			"{output_dir}", opts.OutputDir,
		)
		args = append(args, replacer.Replace(value))
	}
	command := exec.CommandContext(ctx, opts.ScreenshotCommand[0], args...)
	var err error
	command.Env, err = excelCompatAdapterCommandEnv(opts.Environment)
	if err != nil {
		return err
	}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("capture %s screenshots: %s: %w", stage, strings.TrimSpace(string(output)), err)
	}
	if _, err := os.Stat(filepath.Join(opts.OutputDir, stage, "manifest.json")); err != nil {
		return fmt.Errorf("capture %s screenshots produced no manifest: %w", stage, err)
	}
	manifest, err := loadExcelCompatStageManifest(opts.OutputDir, stage, token, "", true)
	if err != nil {
		return fmt.Errorf("validate %s screenshot manifest: %w", stage, err)
	}
	if err := validateExcelCompatManifestSheets(stage, manifest, expectedSheets); err != nil {
		return err
	}
	return nil
}

func runExcelCompatRender(ctx context.Context, opts excelCompatRoundTripOptions, workbook, stage string) error {
	expectedSheets, err := excelCompatWorkbookSheets(workbook)
	if err != nil {
		return err
	}
	if err := resetExcelCompatStageDir(opts.OutputDir, stage); err != nil {
		return err
	}
	args := make([]string, 0, len(opts.RenderCommand)-1)
	for _, value := range opts.RenderCommand[1:] {
		replacer := strings.NewReplacer(
			"{workbook}", workbook,
			"{stage}", stage,
			"{output_dir}", opts.OutputDir,
		)
		args = append(args, replacer.Replace(value))
	}
	command := exec.CommandContext(ctx, opts.RenderCommand[0], args...)
	err = nil
	command.Env, err = excelCompatAdapterCommandEnv(opts.Environment)
	if err != nil {
		return err
	}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("render %s screenshots: %s: %w", stage, strings.TrimSpace(string(output)), err)
	}
	if _, err := os.Stat(filepath.Join(opts.OutputDir, stage, "manifest.json")); err != nil {
		return fmt.Errorf("render %s screenshots produced no manifest: %w", stage, err)
	}
	manifest, err := loadExcelCompatStageManifest(opts.OutputDir, stage, "", workbook, false)
	if err != nil {
		return fmt.Errorf("validate %s render manifest: %w", stage, err)
	}
	if err := validateExcelCompatManifestSheets(stage, manifest, expectedSheets); err != nil {
		return err
	}
	return nil
}

func resetExcelCompatStageDir(outputDir, stage string) error {
	switch stage {
	case "source", "imported", "exported", "reimported":
	default:
		return fmt.Errorf("unsupported screenshot stage %q", stage)
	}
	stageDir := filepath.Join(outputDir, stage)
	relative, err := filepath.Rel(outputDir, stageDir)
	if err != nil || relative != stage {
		return fmt.Errorf("resolve screenshot stage directory %q", stage)
	}
	if err := os.RemoveAll(stageDir); err != nil {
		return fmt.Errorf("clear %s screenshot stage: %w", stage, err)
	}
	return nil
}

func resetExcelCompatOwnedArtifacts(outputDir string) error {
	for _, name := range []string{
		"source",
		"imported",
		"exported",
		"reimported",
		"exported.xlsx",
		"roundtrip.json",
	} {
		target := filepath.Join(outputDir, name)
		relative, err := filepath.Rel(outputDir, target)
		if err != nil || relative != name {
			return fmt.Errorf("resolve owned artifact %q", name)
		}
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove stale artifact %q: %w", name, err)
		}
	}
	return nil
}

func excelCompatCommandEnv(overrides map[string]string) ([]string, error) {
	return excelCompatCommandEnvFrom(excelCompatMinimalBaseEnv(), overrides)
}

func excelCompatCommandEnvFrom(base []string, overrides map[string]string) ([]string, error) {
	envByKey := make(map[string]string, len(base)+len(overrides))
	order := make([]string, 0, len(base)+len(overrides))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			continue
		}
		if _, exists := envByKey[key]; !exists {
			order = append(order, key)
		}
		envByKey[key] = item
	}
	overrideKeys := make([]string, 0, len(overrides))
	for key := range overrides {
		if key == "" || strings.ContainsAny(key, "=\x00") {
			return nil, fmt.Errorf("invalid environment key %q", key)
		}
		overrideKeys = append(overrideKeys, key)
	}
	sort.Strings(overrideKeys)
	for _, key := range overrideKeys {
		if _, exists := envByKey[key]; !exists {
			order = append(order, key)
		}
		envByKey[key] = key + "=" + overrides[key]
	}
	result := make([]string, 0, len(order))
	for _, key := range order {
		result = append(result, envByKey[key])
	}
	return result, nil
}

func excelCompatCLICommandEnv(
	shared map[string]string,
	isolated map[string]string,
) ([]string, error) {
	overrides := make(map[string]string, len(isolated))
	for key, value := range shared {
		if excelCompatExplicitEnvironmentAllowed(key) {
			overrides[key] = value
		}
	}
	for key, value := range isolated {
		overrides[key] = value
	}
	return excelCompatCommandEnvFrom(excelCompatMinimalBaseEnv(), overrides)
}

func excelCompatAdapterCommandEnv(overrides map[string]string) ([]string, error) {
	explicit := make(map[string]string, len(overrides))
	for key, value := range overrides {
		if excelCompatExplicitEnvironmentAllowed(key) {
			explicit[key] = value
		}
	}
	return excelCompatCommandEnvFrom(excelCompatMinimalBaseEnv(), explicit)
}

func excelCompatExplicitEnvironmentAllowed(key string) bool {
	switch key {
	case "LARKSUITE_CLI_APP_ID",
		"LARKSUITE_CLI_APP_SECRET",
		"LARKSUITE_CLI_USER_ACCESS_TOKEN",
		"LARKSUITE_CLI_TENANT_ACCESS_TOKEN",
		"LARKSUITE_CLI_TENANT_ACCESS_TOKEN_SOURCE",
		"LARKSUITE_CLI_PROFILE",
		"LARKSUITE_CLI_DEFAULT_AS",
		"LARKSUITE_CLI_AUTH_PROXY",
		"LARKSUITE_CLI_PROXY_KEY",
		"LARKSUITE_CLI_PROXY_ENABLE",
		"LARKSUITE_CLI_PROXY_ADDRESS",
		"LARKSUITE_CLI_CA_PATH",
		"LARKSUITE_CLI_STRICT_MODE",
		"LARKSUITE_CLI_DATA_DIR",
		"TEST_BOT1_APP_ID",
		"TEST_TENANT_ACCESS_TOKEN",
		"TEST_USER_ACCESS_TOKEN",
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"ALL_PROXY",
		"NO_PROXY",
		"http_proxy",
		"https_proxy",
		"all_proxy",
		"no_proxy":
		return false
	default:
		return !strings.HasPrefix(key, "LARK_CLI_NO_PROXY")
	}
}

func excelCompatMinimalBaseEnv() []string {
	allowed := map[string]struct{}{
		"PATH": {}, "TMPDIR": {}, "TMP": {}, "TEMP": {}, "TZ": {},
		"LANG": {}, "LC_ALL": {}, "DISPLAY": {}, "WAYLAND_DISPLAY": {},
		"XDG_RUNTIME_DIR": {}, "SYSTEMROOT": {}, "WINDIR": {},
		"COMSPEC": {}, "PATHEXT": {},
	}
	result := make([]string, 0, len(allowed))
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, keep := allowed[key]; keep {
			result = append(result, item)
		}
	}
	return result
}

func validateExcelCompatRepositoryBinary(binary, repositoryRoot string) error {
	binaryPath, err := filepath.Abs(binary)
	if err != nil {
		return fmt.Errorf("resolve lark-cli binary: %w", err)
	}
	expected, err := filepath.Abs(filepath.Join(repositoryRoot, "lark-cli"))
	if err != nil {
		return fmt.Errorf("resolve repository lark-cli binary: %w", err)
	}
	if binaryPath != expected {
		return fmt.Errorf(
			"Excel compatibility workflow requires repository-local %s; resolved %s",
			expected, binaryPath,
		)
	}
	info, err := os.Lstat(binaryPath)
	if err != nil {
		return fmt.Errorf("inspect repository-local lark-cli: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("repository-local lark-cli must not be a symlink")
	}
	if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return errors.New("repository-local lark-cli must be an executable regular file")
	}
	return nil
}

func findExcelCompatRepositoryRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		goMod := filepath.Join(current, "go.mod")
		data, readErr := os.ReadFile(goMod)
		if readErr == nil && strings.Contains(
			string(data), "module github.com/larksuite/cli",
		) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("lark-cli repository root is unavailable")
		}
		current = parent
	}
}

func loadExcelCompatStageManifest(
	outputDir string,
	stage string,
	expectedToken string,
	expectedWorkbook string,
	requireReal bool,
) (excelCompatStageManifest, error) {
	var manifest excelCompatStageManifest
	stageDir := filepath.Join(outputDir, stage)
	stageInfo, err := os.Lstat(stageDir)
	if err != nil {
		return manifest, err
	}
	if stageInfo.Mode()&os.ModeSymlink != 0 || !stageInfo.IsDir() {
		return manifest, fmt.Errorf("%s stage directory must be a real directory", stage)
	}
	manifestPath := filepath.Join(stageDir, "manifest.json")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil {
		return manifest, err
	}
	if manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular() {
		return manifest, fmt.Errorf("%s manifest must be a regular non-symlink file", stage)
	}
	file, err := os.Open(manifestPath)
	if err != nil {
		return manifest, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	if err != nil {
		return manifest, fmt.Errorf("read manifest: %w", err)
	}
	if len(data) > 4<<20 {
		return manifest, errors.New("manifest exceeds 4 MiB")
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.Stage != stage {
		return manifest, fmt.Errorf("manifest stage %q does not match %q", manifest.Stage, stage)
	}
	if len(manifest.Screenshots) == 0 {
		return manifest, fmt.Errorf("%s manifest contains no screenshots", stage)
	}
	if requireReal {
		if manifest.Provenance.Kind != "lark_sheet" {
			return manifest, fmt.Errorf("%s requires real screenshot provenance kind lark_sheet", stage)
		}
		if manifest.Token != expectedToken {
			return manifest, fmt.Errorf("%s manifest token does not match the captured sheet", stage)
		}
	} else {
		if manifest.Provenance.Kind != "synthetic_preview" {
			return manifest, fmt.Errorf("%s requires synthetic_preview provenance", stage)
		}
		matches, err := sameExcelCompatFile(manifest.Workbook, expectedWorkbook)
		if err != nil {
			return manifest, fmt.Errorf("%s workbook provenance: %w", stage, err)
		}
		if !matches {
			return manifest, fmt.Errorf("%s manifest workbook does not match the rendered workbook", stage)
		}
	}
	seenIndexes := make(map[int]struct{}, len(manifest.Screenshots))
	seenNames := make(map[string]struct{}, len(manifest.Screenshots))
	seenFiles := make(map[string]struct{}, len(manifest.Screenshots))
	for _, screenshot := range manifest.Screenshots {
		if screenshot.SheetIndex < 0 {
			return manifest, fmt.Errorf("%s has negative sheet_index", stage)
		}
		if strings.TrimSpace(screenshot.SheetName) == "" {
			return manifest, fmt.Errorf("%s has an empty sheet_name", stage)
		}
		if screenshot.Stage != stage {
			return manifest, fmt.Errorf(
				"%s screenshot for %q declares stage %q",
				stage, screenshot.SheetName, screenshot.Stage,
			)
		}
		if _, exists := seenIndexes[screenshot.SheetIndex]; exists {
			return manifest, fmt.Errorf("%s has duplicate sheet_index %d", stage, screenshot.SheetIndex)
		}
		if _, exists := seenNames[screenshot.SheetName]; exists {
			return manifest, fmt.Errorf("%s has duplicate sheet_name %q", stage, screenshot.SheetName)
		}
		seenIndexes[screenshot.SheetIndex] = struct{}{}
		seenNames[screenshot.SheetName] = struct{}{}

		imagePath, err := excelCompatScreenshotPath(outputDir, stage, screenshot.File)
		if err != nil {
			return manifest, err
		}
		if strings.ToLower(filepath.Ext(imagePath)) != ".png" {
			return manifest, fmt.Errorf("%s screenshot %q is not PNG", stage, screenshot.File)
		}
		if _, exists := seenFiles[imagePath]; exists {
			return manifest, fmt.Errorf("%s has duplicate screenshot file %q", stage, screenshot.File)
		}
		seenFiles[imagePath] = struct{}{}
		info, err := os.Lstat(imagePath)
		if err != nil {
			return manifest, fmt.Errorf("%s screenshot %q is unavailable: %w", stage, screenshot.File, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return manifest, fmt.Errorf("%s screenshot %q must not be a symlink", stage, screenshot.File)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return manifest, fmt.Errorf("%s screenshot %q is not a non-empty file", stage, screenshot.File)
		}
		imageFile, err := os.Open(imagePath)
		if err != nil {
			return manifest, fmt.Errorf("%s screenshot %q cannot be opened: %w", stage, screenshot.File, err)
		}
		config, format, decodeErr := image.DecodeConfig(io.LimitReader(imageFile, 16<<20))
		closeErr := imageFile.Close()
		if decodeErr != nil || format != "png" {
			return manifest, fmt.Errorf("%s screenshot %q is not a valid PNG: %w", stage, screenshot.File, decodeErr)
		}
		if closeErr != nil {
			return manifest, fmt.Errorf("%s screenshot %q cannot be closed: %w", stage, screenshot.File, closeErr)
		}
		if config.Width <= 0 || config.Height <= 0 ||
			config.Width > excelCompatMaxPNGDimension ||
			config.Height > excelCompatMaxPNGDimension ||
			config.Width > excelCompatMaxPNGPixelCount/config.Height {
			return manifest, fmt.Errorf(
				"%s screenshot %q exceeds image dimensions",
				stage, screenshot.File,
			)
		}
		imageFile, err = os.Open(imagePath)
		if err != nil {
			return manifest, fmt.Errorf("%s screenshot %q cannot be reopened: %w", stage, screenshot.File, err)
		}
		decoded, _, decodeErr := image.Decode(io.LimitReader(imageFile, 16<<20))
		closeErr = imageFile.Close()
		if decodeErr != nil || decoded == nil {
			return manifest, fmt.Errorf("%s screenshot %q is truncated or corrupt: %w", stage, screenshot.File, decodeErr)
		}
		if closeErr != nil {
			return manifest, fmt.Errorf("%s screenshot %q cannot be closed: %w", stage, screenshot.File, closeErr)
		}
		realImagePath, err := filepath.EvalSymlinks(imagePath)
		if err != nil {
			return manifest, fmt.Errorf("%s screenshot %q cannot be resolved: %w", stage, screenshot.File, err)
		}
		realStageDir, err := filepath.EvalSymlinks(stageDir)
		if err != nil {
			return manifest, fmt.Errorf("%s stage directory cannot be resolved: %w", stage, err)
		}
		relative, err := filepath.Rel(realStageDir, realImagePath)
		if err != nil ||
			relative == ".." ||
			strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return manifest, fmt.Errorf("%s screenshot %q resolves outside its stage directory", stage, screenshot.File)
		}
	}
	return manifest, nil
}

func validateExcelCompatManifestSheets(
	stage string,
	manifest excelCompatStageManifest,
	expected []excelCompatSheetIdentity,
) error {
	actual := make([]excelCompatSheetIdentity, 0, len(manifest.Screenshots))
	for _, screenshot := range manifest.Screenshots {
		actual = append(actual, excelCompatSheetIdentity{
			Index: screenshot.SheetIndex,
			Name:  screenshot.SheetName,
		})
	}
	if len(actual) != len(expected) {
		return fmt.Errorf(
			"%s manifest has %d sheets; workbook has %d",
			stage, len(actual), len(expected),
		)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			return fmt.Errorf(
				"%s sheet %d is %+v; workbook requires %+v",
				stage, index, actual[index], expected[index],
			)
		}
	}
	return nil
}

func sameExcelCompatFile(left, right string) (bool, error) {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false, errors.New("workbook path is empty")
	}
	leftPath, err := filepath.EvalSymlinks(left)
	if err != nil {
		return false, err
	}
	rightPath, err := filepath.EvalSymlinks(right)
	if err != nil {
		return false, err
	}
	leftPath, err = filepath.Abs(leftPath)
	if err != nil {
		return false, err
	}
	rightPath, err = filepath.Abs(rightPath)
	if err != nil {
		return false, err
	}
	return leftPath == rightPath, nil
}

func excelCompatScreenshotPath(outputDir, stage, value string) (string, error) {
	if value == "" || filepath.IsAbs(value) {
		return "", fmt.Errorf("%s screenshot path must be relative", stage)
	}
	clean := filepath.Clean(filepath.FromSlash(value))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s screenshot path escapes the output directory", stage)
	}
	if filepath.Dir(clean) == "." {
		clean = filepath.Join(stage, clean)
	}
	expectedPrefix := stage + string(filepath.Separator)
	if clean != stage && !strings.HasPrefix(clean, expectedPrefix) {
		return "", fmt.Errorf("%s screenshot path %q belongs to another stage", stage, value)
	}
	return filepath.Join(outputDir, clean), nil
}

func validateExcelCompatStageSet(
	outputDir string,
	sourceWorkbook string,
	importedToken string,
	exportedWorkbook string,
	reimportedToken string,
) error {
	sourceSheets, err := excelCompatWorkbookSheets(sourceWorkbook)
	if err != nil {
		return err
	}
	exportedSheets, err := excelCompatWorkbookSheets(exportedWorkbook)
	if err != nil {
		return err
	}
	if len(sourceSheets) != len(exportedSheets) {
		return errors.New("exported workbook sheet set differs from source workbook")
	}
	for index := range sourceSheets {
		if sourceSheets[index] != exportedSheets[index] {
			return errors.New("exported workbook sheet set differs from source workbook")
		}
	}
	stages := []struct {
		name        string
		token       string
		workbook    string
		sheets      []excelCompatSheetIdentity
		requireReal bool
	}{
		{name: "source", workbook: sourceWorkbook, sheets: sourceSheets},
		{name: "imported", token: importedToken, sheets: sourceSheets, requireReal: true},
		{name: "exported", workbook: exportedWorkbook, sheets: exportedSheets},
		{name: "reimported", token: reimportedToken, sheets: exportedSheets, requireReal: true},
	}
	for _, stage := range stages {
		manifest, err := loadExcelCompatStageManifest(
			outputDir,
			stage.name,
			stage.token,
			stage.workbook,
			stage.requireReal,
		)
		if err != nil {
			return fmt.Errorf("%s: %w", stage.name, err)
		}
		if err := validateExcelCompatManifestSheets(stage.name, manifest, stage.sheets); err != nil {
			return err
		}
	}
	return nil
}
