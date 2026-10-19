package auth

import (
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// NewCmdAuthConfirmProtocol explicitly confirms one existing account's protocol;
// it does not log in, move credentials across authorities, or clear uncertain refreshes.
func NewCmdAuthConfirmProtocol(f *cmdutil.Factory) *cobra.Command {
	var protocol, user string
	cmd := &cobra.Command{Use: "confirm-protocol", Short: "Confirm an existing account's authentication protocol without reauthorization", RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := f.Config()
		if err != nil {
			return err
		}
		if user == "" {
			user = cfg.UserOpenId
		}
		if user == "" {
			return output.ErrValidation("--user-id required for an existing account")
		}
		ep, err := cfg.ResolveServiceEndpoints()
		if err != nil {
			return err
		}
		if err = larkauth.ConfirmCredentialProtocol(cmd.Context(), nil, cfg.AppID, user, protocol, ep.Open); err != nil {
			return output.ErrAuth("protocol confirmation failed: %v", err)
		}
		output.PrintSuccess(f.IOStreams.ErrOut, "Account protocol confirmed; credentials preserved")
		return nil
	}}
	cmd.Flags().StringVar(&protocol, "auth-protocol", "", "legacy_oidc or oauth_v2")
	cmd.Flags().StringVar(&user, "user-id", "", "existing user open_id (defaults to selected user)")
	_ = cmd.MarkFlagRequired("auth-protocol")
	return cmd
}
