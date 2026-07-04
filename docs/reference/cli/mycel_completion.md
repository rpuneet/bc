## mycel completion

Generate shell completion scripts

### Synopsis

Generate shell completion scripts for bc.

To load completions:

Bash:
  $ source <(bc completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ bc completion bash > /etc/bash_completion.d/bc
  # macOS:
  $ bc completion bash > $(brew --prefix)/etc/bash_completion.d/bc

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ bc completion zsh > "${fpath[1]}/_bc"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ bc completion fish | source

  # To load completions for each session, execute once:
  $ bc completion fish > ~/.config/fish/completions/bc.fish

PowerShell:
  PS> bc completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> bc completion powershell > bc.ps1
  # and source this file from your PowerShell profile.


```
mycel completion [bash|zsh|fish|powershell]
```

### Options

```
  -h, --help   help for completion
```

### Options inherited from parent commands

```
      --json      Output in JSON format
  -v, --verbose   Enable verbose output
```

### SEE ALSO

* [mycel](mycel.md)	 - A simpler, more controllable agent orchestrator

