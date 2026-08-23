package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/After-Certainty/pade/internal/binding"
	"github.com/After-Certainty/pade/internal/execution"
	"github.com/After-Certainty/pade/internal/manifest"
	"github.com/After-Certainty/pade/internal/output"
	"github.com/After-Certainty/pade/internal/planner"
	"github.com/After-Certainty/pade/internal/providerset"
	"github.com/After-Certainty/pade/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:           "pade",
		Short:         "Portable Agent Development Environments CLI",
		Long:          "PADE validates and plans portable capability declarations for agent development environments. Workspace lifecycle is owned by DevPod (or equivalent).",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.String(),
	}

	var (
		file     string
		bindings string
		jsonOut  bool
	)

	root.PersistentFlags().StringVarP(&file, "file", "f", "", "path to pade.yaml (default: ./pade.yaml)")
	root.PersistentFlags().StringVar(&bindings, "bindings", "", "path to trusted local bindings.yaml (default: PADE_BINDINGS, then ~/.config/pade/bindings.yaml; workspace .pade/bindings.yaml only with PADE_TRUST_WORKSPACE_BINDINGS=1)")
	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")

	root.AddCommand(newValidateCmd(&file, &jsonOut))
	root.AddCommand(newPlanCmd(&file, &bindings, &jsonOut))
	root.AddCommand(newCapabilitiesCmd(&file, &bindings, &jsonOut))
	root.AddCommand(newExecCmd(&file, &bindings))
	root.AddCommand(newIdentityCmd(&jsonOut))

	if err := root.Execute(); err != nil {
		var ee *execution.ExitError
		if errors.As(err, &ee) {
			os.Exit(ee.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newValidateCmd(file *string, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate pade.yaml against the PADE schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, res, err := loadAndValidate(*file)
			if err != nil {
				return err
			}
			if *jsonOut {
				if err := output.WriteJSON(cmd.OutOrStdout(), res); err != nil {
					return err
				}
			} else {
				output.WriteValidateHuman(cmd.OutOrStdout(), res)
			}
			if !res.Valid {
				return fmt.Errorf("validation failed")
			}
			return nil
		},
	}
}

func newPlanCmd(file, bindings *string, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Show a side-effect-free execution plan for the manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, res, err := loadAndValidate(*file)
			if err != nil {
				return err
			}
			if !res.Valid {
				if !*jsonOut {
					output.WriteValidateHuman(cmd.OutOrStdout(), res)
				}
				return fmt.Errorf("validation failed; fix the manifest before planning")
			}
			cfg, statuses, err := resolveBindings(cmd.Context(), m, *bindings)
			if err != nil {
				return err
			}
			plan := planner.Build(m, planner.BuildOptions{Bindings: cfg, Statuses: statuses})
			if *jsonOut {
				return output.WriteJSON(cmd.OutOrStdout(), plan)
			}
			output.WritePlanHuman(cmd.OutOrStdout(), plan)
			return nil
		},
	}
}

func newCapabilitiesCmd(file, bindings *string, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "capabilities",
		Short: "Show declared capabilities and local binding status (never secret values)",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, res, err := loadAndValidate(*file)
			if err != nil {
				return err
			}
			if !res.Valid {
				if !*jsonOut {
					output.WriteValidateHuman(cmd.OutOrStdout(), res)
				}
				return fmt.Errorf("validation failed; fix the manifest before inspecting capabilities")
			}
			cfg, statuses, err := resolveBindings(cmd.Context(), m, *bindings)
			if err != nil {
				return err
			}
			path := ""
			if cfg != nil {
				path = cfg.SourcePath
			}
			if *jsonOut {
				return output.WriteJSON(cmd.OutOrStdout(), map[string]any{
					"bindingsPath": path,
					"capabilities": statuses,
				})
			}
			output.WriteCapabilitiesHuman(cmd.OutOrStdout(), statuses, path)
			return nil
		},
	}
}

func newExecCmd(file, bindings *string) *cobra.Command {
	var capabilities []string
	var quiet bool
	cmd := &cobra.Command{
		Use:   "exec --capability NAME -- COMMAND [ARGS...]",
		Short: "Run a command with process-scoped capability credentials",
		Long: `Resolve one or more declared capabilities and inject their credentials only into the child process.

Secret values are never printed. After the command exits, resolved material is discarded from PADE's memory maps (the child process may still have observed them while running).

Exact resolved secret values are best-effort redacted from child stdout/stderr before they reach the caller. Redaction is defense in depth, not a security boundary.

Example:
  pade exec --capability github.user.read -- ./scripts/github-whoami`,
		DisableFlagsInUseLine: true,
		Args:                  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(capabilities) == 0 {
				return fmt.Errorf("at least one --capability is required")
			}
			m, res, err := loadAndValidate(*file)
			if err != nil {
				return err
			}
			if !res.Valid {
				output.WriteValidateHuman(cmd.ErrOrStderr(), res)
				return fmt.Errorf("validation failed; fix the manifest before exec")
			}
			for _, name := range capabilities {
				if _, ok := m.Spec.Capabilities[name]; !ok {
					return fmt.Errorf("capability %q is not declared in the manifest", name)
				}
			}
			cfg, err := binding.LoadOptional(filepath.Dir(m.SourcePath), *bindings)
			if err != nil {
				return err
			}
			if cfg.SourcePath == "" {
				return fmt.Errorf("no bindings file found; configure --bindings, PADE_BINDINGS, ~/.config/pade/bindings.yaml, or set PADE_TRUST_WORKSPACE_BINDINGS=1 for .pade/bindings.yaml")
			}
			reg := providerset.Consumer()
			runner := &execution.Runner{Registry: reg}
			_, err = runner.Run(cmd.Context(), cfg, capabilities, execution.Options{
				Command: args,
				Dir:     filepath.Dir(m.SourcePath),
				Stdout:  cmd.OutOrStdout(),
				Stderr:  cmd.ErrOrStderr(),
				Stdin:   cmd.InOrStdin(),
				Quiet:   quiet,
			})
			return err
		},
	}
	cmd.Flags().StringArrayVarP(&capabilities, "capability", "c", nil, "capability to resolve into the child process (repeatable)")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "suppress the non-secret capability injection notice on stderr")
	return cmd
}

func loadAndValidate(file string) (*manifest.Manifest, *manifest.Result, error) {
	path, err := manifest.Find("", file)
	if err != nil {
		return nil, nil, err
	}
	m, err := manifest.Load(path)
	if err != nil {
		return nil, nil, err
	}
	res, err := manifest.Validate(m)
	if err != nil {
		return nil, nil, err
	}
	return m, res, nil
}

func resolveBindings(_ context.Context, m *manifest.Manifest, bindingsPath string) (*binding.Config, []binding.Status, error) {
	cfg, err := binding.LoadOptional(filepath.Dir(m.SourcePath), bindingsPath)
	if err != nil {
		return nil, nil, err
	}
	reg := providerset.Consumer()
	views := map[string]binding.CapabilityRequestView{}
	for name, cap := range m.Spec.Capabilities {
		views[name] = binding.CapabilityRequestView{
			Access:   cap.Access,
			Required: cap.IsRequired(),
		}
	}
	// Static inspection only: plan/capabilities must not Probe or Resolve.
	statuses := binding.InspectBindings(reg, views, cfg)
	return cfg, statuses, nil
}
