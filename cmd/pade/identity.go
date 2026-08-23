package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	cursorid "github.com/After-Certainty/pade/internal/identity/cursor"
	"github.com/spf13/cobra"
)

func newIdentityCmd(jsonOut *bool) *cobra.Command {
	var audience string
	cmd := &cobra.Command{
		Use:   "identity",
		Short: "Inspect Cursor Cloud Agent workload identity (safe claims only)",
		Long: `Mint a Cursor Cloud Agent OIDC token and print safe decoded claims.

The raw JWT is never printed. This proves PADE can obtain Cursor workload
identity; it does not authorize capabilities by itself.

Requires a Cursor Cloud Agent identity socket (CURSOR_AGENT_SOCKET or
/run/cursor/api.sock).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			if audience == "" {
				return fmt.Errorf("--audience is required")
			}
			src := cursorid.New()
			tok, err := src.Token(cmd.Context(), audience)
			if err != nil {
				return err
			}
			claims, err := cursorid.DecodeSafeClaims(tok.Value)
			if err != nil {
				return err
			}
			if claims.ExpiresAt == "" && !tok.ExpiresAt.IsZero() {
				claims.ExpiresAt = tok.ExpiresAt.UTC().Format(time.RFC3339)
			}
			if *jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(claims)
			}
			return writeIdentityHuman(cmd.OutOrStdout(), claims)
		},
	}
	cmd.Flags().StringVar(&audience, "audience", "", "OIDC audience to request (required)")
	return cmd
}

func writeIdentityHuman(w io.Writer, claims cursorid.SafeClaims) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "provider:\t%s\n", claims.Provider)
	if claims.Subject != "" {
		fmt.Fprintf(tw, "subject:\t%s\n", claims.Subject)
	}
	if claims.OwnerUserID != "" {
		fmt.Fprintf(tw, "owner_user_id:\t%s\n", claims.OwnerUserID)
	}
	if claims.CloudAgentID != "" {
		fmt.Fprintf(tw, "cloud_agent:\t%s\n", claims.CloudAgentID)
	}
	if claims.AgentRuntime != "" {
		fmt.Fprintf(tw, "runtime:\t%s\n", claims.AgentRuntime)
	}
	if claims.TeamID != "" {
		fmt.Fprintf(tw, "team_id:\t%s\n", claims.TeamID)
	}
	if claims.Audience != "" {
		fmt.Fprintf(tw, "audience:\t%s\n", claims.Audience)
	}
	if claims.EnvironmentID != "" {
		fmt.Fprintf(tw, "environment:\t%s\n", claims.EnvironmentID)
	}
	if claims.Source != "" {
		fmt.Fprintf(tw, "source:\t%s\n", claims.Source)
	}
	if claims.RepoCount > 0 {
		fmt.Fprintf(tw, "repo_count:\t%d\n", claims.RepoCount)
	}
	if len(claims.RepoURLs) > 0 {
		fmt.Fprintf(tw, "repos:\n")
		for _, r := range claims.RepoURLs {
			fmt.Fprintf(tw, "  - %s\n", r)
		}
	} else if claims.RepoURL != "" {
		fmt.Fprintf(tw, "repo_url (primary only):\t%s\n", claims.RepoURL)
		fmt.Fprintf(tw, "note:\trepo_url alone does not prove single-repo confinement\n")
	}
	if claims.ExpiresAt != "" {
		fmt.Fprintf(tw, "expires:\t%s\n", claims.ExpiresAt)
	}
	fmt.Fprintf(tw, "note:\traw JWT not printed; token authenticates the Cloud Agent workload, not a subprocess\n")
	return tw.Flush()
}
