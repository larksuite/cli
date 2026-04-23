// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// DetectFileFields returns field names with type "file" in the method's requestBody.
func DetectFileFields(method map[string]interface{}) []string {
	rb, _ := method["requestBody"].(map[string]interface{})
	var fields []string
	for name, field := range rb {
		f, _ := field.(map[string]interface{})
		if registry.GetStrFromMap(f, "type") == "file" {
			fields = append(fields, name)
		}
	}
	return fields
}

// ParseFileFlag parses a --file flag value into its components.
// The format is either "path" or "field=path". When no explicit "field="
// prefix is present, defaultField is used as the field name.
// A path of "-" indicates stdin; in that case filePath is empty and isStdin is true.
func ParseFileFlag(raw, defaultField string) (fieldName, filePath string, isStdin bool) {
	if idx := strings.IndexByte(raw, '='); idx > 0 {
		fieldName = raw[:idx]
		filePath = raw[idx+1:]
	} else {
		fieldName = defaultField
		filePath = raw
	}
	if filePath == "-" {
		return fieldName, "", true
	}
	return fieldName, filePath, false
}

// ValidateFileFlag checks mutual exclusion rules for the --file flag.
// Returns nil if file is empty (flag not provided).
func ValidateFileFlag(file, params, data, outputPath string, pageAll bool, httpMethod string) error {
	if file == "" {
		return nil
	}

	_, filePath, isStdin := ParseFileFlag(file, "file")
	if !isStdin && filePath == "" {
		return output.ErrValidation("--file: empty file path")
	}

	if outputPath != "" {
		return output.ErrValidation("--file and --output are mutually exclusive")
	}
	if pageAll {
		return output.ErrValidation("--file and --page-all are mutually exclusive")
	}
	if isStdin && data == "-" {
		return output.ErrValidation("--file and --data cannot both read from stdin")
	}
	if isStdin && params == "-" {
		return output.ErrValidation("--file and --params cannot both read from stdin")
	}

	switch httpMethod {
	case "POST", "PUT", "PATCH", "DELETE":
	default:
		return output.ErrValidation("--file requires POST, PUT, PATCH, or DELETE method")
	}

	return nil
}

// FileUploadMeta holds file upload metadata for dry-run display.
// Returned by request builders when dry-run mode skips actual file reading.
type FileUploadMeta struct {
	FieldName  string
	FilePath   string
	FormFields any
}

// BuildFormdata constructs a multipart form data payload for file upload.
// If isStdin is true, the file content is read from stdin.
// Top-level keys from dataJSON are added as text form fields.
func BuildFormdata(fileIO fileio.FileIO, fieldName, filePath string, isStdin bool, stdin io.Reader, dataJSON any) (*larkcore.Formdata, error) {
	fd := larkcore.NewFormdata()

	if isStdin {
		if stdin == nil {
			return nil, output.ErrValidation("--file: stdin is not available")
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, output.ErrValidation("--file: failed to read stdin: %v", err)
		}
		if len(data) == 0 {
			return nil, output.ErrValidation("--file: stdin is empty")
		}
		fd.AddFile(fieldName, bytes.NewReader(data))
	} else {
		f, err := fileIO.Open(filePath)
		if err != nil {
			return nil, output.ErrValidation("cannot open file: %s", filePath)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			return nil, output.ErrValidation("--file: failed to read %s: %v", filePath, err)
		}
		fd.AddFile(fieldName, bytes.NewReader(data))
	}

	// Add top-level JSON keys as text form fields.
	if m, ok := dataJSON.(map[string]any); ok {
		for k, v := range m {
			fd.AddField(k, formatFormFieldValue(v))
		}
	}

	return fd, nil
}

// formatFormFieldValue renders a JSON-unmarshalled value as a multipart form
// field string. float64 is handled specially: fmt's default %v/%g switches to
// scientific notation for values >= ~1e6 (e.g. "1.185356e+06"), which some
// backends reject when parsing the field as an integer. Use decimal notation
// instead so size / block_num / offset-style numeric fields round-trip cleanly.
// All other types fall through to %v.
func formatFormFieldValue(v any) string {
	if n, ok := v.(float64); ok {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return fmt.Sprintf("%v", v)
}
