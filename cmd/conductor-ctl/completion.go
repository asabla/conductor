package main

import (
	"os"

	"github.com/spf13/cobra"
)

// completionCmd is the parent command for shell completion
var completionCmd = &cobra.Command{
	Use:   "completion",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for conductor-ctl.

To load completions:

Bash:
  $ source <(conductor-ctl completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ conductor-ctl completion bash > /etc/bash_completion.d/conductor-ctl
  # macOS:
  $ conductor-ctl completion bash > $(brew --prefix)/etc/bash_completion.d/conductor-ctl

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ conductor-ctl completion zsh > "${fpath[1]}/_conductor-ctl"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ conductor-ctl completion fish | source

  # To load completions for each session, execute once:
  $ conductor-ctl completion fish > ~/.config/fish/completions/conductor-ctl.fish

PowerShell:
  PS> conductor-ctl completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> conductor-ctl completion powershell > conductor-ctl.ps1
  # and source this file from your PowerShell profile.
`,
}

// completionBashCmd generates bash completion
var completionBashCmd = &cobra.Command{
	Use:   "bash",
	Short: "Generate bash completion script",
	Long: `Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:
  $ source <(conductor-ctl completion bash)

To load completions for every new session, execute once:
  # Linux:
  $ conductor-ctl completion bash > /etc/bash_completion.d/conductor-ctl

  # macOS:
  $ conductor-ctl completion bash > $(brew --prefix)/etc/bash_completion.d/conductor-ctl

You will need to start a new shell for this setup to take effect.`,
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rootCmd.GenBashCompletion(os.Stdout)
	},
}

// completionZshCmd generates zsh completion
var completionZshCmd = &cobra.Command{
	Use:   "zsh",
	Short: "Generate zsh completion script",
	Long: `Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it. You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:
  $ source <(conductor-ctl completion zsh)

To load completions for every new session, execute once:
  $ conductor-ctl completion zsh > "${fpath[1]}/_conductor-ctl"

You will need to start a new shell for this setup to take effect.`,
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rootCmd.GenZshCompletion(os.Stdout)
	},
}

// completionFishCmd generates fish completion
var completionFishCmd = &cobra.Command{
	Use:   "fish",
	Short: "Generate fish completion script",
	Long: `Generate the autocompletion script for the fish shell.

To load completions in your current shell session:
  $ conductor-ctl completion fish | source

To load completions for every new session, execute once:
  $ conductor-ctl completion fish > ~/.config/fish/completions/conductor-ctl.fish

You will need to start a new shell for this setup to take effect.`,
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rootCmd.GenFishCompletion(os.Stdout, true)
	},
}

// completionPowershellCmd generates powershell completion
var completionPowershellCmd = &cobra.Command{
	Use:   "powershell",
	Short: "Generate powershell completion script",
	Long: `Generate the autocompletion script for powershell.

To load completions in your current shell session:
  PS> conductor-ctl completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.`,
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
	},
}

func init() {
	completionCmd.AddCommand(completionBashCmd)
	completionCmd.AddCommand(completionZshCmd)
	completionCmd.AddCommand(completionFishCmd)
	completionCmd.AddCommand(completionPowershellCmd)
}
