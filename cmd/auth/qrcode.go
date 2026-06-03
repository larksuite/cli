// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/skip2/go-qrcode"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

// QRCodeOptions holds inputs for auth qrcode command.
type QRCodeOptions struct {
	Factory *cmdutil.Factory
	Ctx     context.Context
	URL     string
	Size    int
	ASCII   bool
	Output  string
}

// NewCmdAuthQRCode creates the auth qrcode subcommand.
func NewCmdAuthQRCode(f *cmdutil.Factory, runF func(*QRCodeOptions) error) *cobra.Command {
	opts := &QRCodeOptions{Factory: f, Size: 256}

	cmd := &cobra.Command{
		Use:   "qrcode <url>",
		Short: "Generate QR code for verification URL",
		Long: `Generate a QR code image or ASCII representation for a verification URL.

This command is designed for AI agents to generate QR codes for OAuth authorization URLs.

For PNG output, the --output flag is required to specify the output file path (must be a relative path within the current directory).
For ASCII output, the result is printed to stdout with fixed size.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.URL = args[0]
			opts.Ctx = cmd.Context()
			if runF != nil {
				return runF(opts)
			}
			return runQRCode(opts)
		},
	}

	cmd.Flags().IntVar(&opts.Size, "size", 256, "Size of the QR code image in pixels (default: 256, for PNG mode only)")
	cmd.Flags().BoolVar(&opts.ASCII, "ascii", false, "Output ASCII QR code to stdout")
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "Output file path for PNG image (relative path within current directory, required for non-ASCII mode)")

	return cmd
}

// runQRCode executes the auth qrcode command.
func runQRCode(opts *QRCodeOptions) error {
	if opts.URL == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "url is required").WithParam("--url")
	}

	if opts.ASCII {
		var out io.Writer = os.Stdout
		if opts.Factory != nil {
			out = opts.Factory.IOStreams.Out
		}
		return generateASCIIQRCode(opts.URL, out)
	}

	if opts.Output == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "output file path is required for PNG mode. Use --output or -o flag to specify the output file path.").WithParam("--output")
	}

	if opts.Size < 32 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "size must be at least 32, got %d", opts.Size).WithParam("--size")
	}

	if opts.Size > 1024 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "size must be at most 1024, got %d", opts.Size).WithParam("--size")
	}

	safePath, err := validate.SafeOutputPath(opts.Output)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithParam("--output").WithCause(err)
	}

	if err := generateImageQRCode(opts.URL, opts.Size, safePath); err != nil {
		return err
	}

	result := map[string]interface{}{
		"ok":        true,
		"file_path": safePath,
		"hint":      "You MUST include the QR image in your response. Generating the file alone is NOT enough—use image tags, inline images, or file attachments to display it.",
	}

	var out io.Writer = os.Stdout
	if opts.Factory != nil {
		out = opts.Factory.IOStreams.Out
	}
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "failed to write output: %v", err).WithCause(err)
	}

	return nil
}

// generateImageQRCode encodes the URL as a PNG QR code and writes it to outputPath.
func generateImageQRCode(url string, size int, outputPath string) error {
	png, err := qrcode.Encode(url, qrcode.Medium, size)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "failed to encode QR code: %v", err).WithCause(err)
	}

	err = vfs.WriteFile(outputPath, png, 0644)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "failed to write QR code to %s: %v", outputPath, err).WithCause(err)
	}

	return nil
}

// generateASCIIQRCode encodes the URL as an ASCII QR code and prints it to stdout.
func generateASCIIQRCode(url string, w io.Writer) error {
	q, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "failed to create QR code: %v", err).WithCause(err)
	}

	fmt.Fprint(w, q.ToSmallString(false))

	return nil
}
