## mycel completion

Generate shell completion scripts

### Synopsis

Generate shell completion scripts for mycel.

To load completions:

Bash:
  $ source <(mycel completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ mycel completion bash > /etc/bash_completion.d/mycel
  # macOS:
  $ mycel completion bash > $(brew --prefix)/etc/bash_completion.d/mycel

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ mycel completion zsh > "${fpath[1]}/_mycel"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ mycel completion fish | source

  # To load completions for each session, execute once:
  $ mycel completion fish > ~/.config/fish/completions/mycel.fish

PowerShell:
  PS> mycel completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> mycel completion powershell > mycel.ps1
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

