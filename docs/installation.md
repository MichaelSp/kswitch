---
title: Installation
---

# Installation

The kswitch installation consists of a `kubectl-switch` binary and a shell function that must be sourced into your shell.

**NOTE**: Always invoke kswitch via the shell function (`kswitch`), not the binary directly (`kubectl-switch`).
The shell function is required to export `KUBECONFIG` into your current shell session.

## Option 1 - Homebrew (macOS)

```sh
brew tap MichaelSp/kswitch
brew install --cask kswitch
```

Next, follow [required: source the shell function](#required-source-the-shell-function).

## Option 2 - Github releases

Download the `kubectl-switch` binary:
```sh
OS=linux                        # Pick the right os: linux, darwin (intel only)
VERSION=0.5.0                   # Pick the current version.

curl -L -o /usr/local/bin/kubectl-switch https://github.com/MichaelSp/kswitch/releases/download/${VERSION}/kubectl-switch_${OS}_amd64
chmod +x /usr/local/bin/kubectl-switch
```

If you are using Windows, go to the release page and download the Windows binary: <https://github.com/MichaelSp/kswitch/releases/>\
Copy it to a folder in your PATH. To add a folder to the path for the current PowerShell session: `$env:Path += ';C:\myfolder'`

Next, follow [required: source the shell function](#required-source-the-shell-function).

## Option 3 - From source

```
go get github.com/MichaelSp/kswitch
```

From the repository root run `make build-kswitch`.
This builds the binaries to `/hack/switch/`.
Copy the binary for your OS/architecture to e.g. `/usr/local/bin/kubectl-switch`.

Next, follow [required: source the shell function](#required-source-the-shell-function).

## Required: Source the shell function

The `kubectl-switch init` command generates both the `kswitch` shell function and the tab-completion script.
Source its output in your shell rc file so the `kswitch` command is available in every new shell.

### Bash

```sh
echo 'source <(kubectl-switch init bash)' >> ~/.bashrc

# optionally use alias `s` instead of `kswitch`
echo 'alias s=kswitch' >> ~/.bashrc
echo 'complete -o default -F __start_kswitch s' >> ~/.bashrc
```

### Zsh

```sh
echo 'source <(kubectl-switch init zsh)' >> ~/.zshrc

# optionally use alias `s` instead of `kswitch`
echo 'alias s=kswitch' >> ~/.zshrc
```

### Fish

```sh
echo 'kubectl-switch init fish | source' >> ~/.config/fish/config.fish

# optionally use alias `s` instead of `kswitch`
echo 'function s --wraps kswitch; kswitch $argv; end' >> ~/.config/fish/config.fish
```

### PowerShell

```powershell
kubectl-switch.exe init powershell >> $PROFILE

# add this for autocomplete to work
echo 'Register-ArgumentCompleter -CommandName ''kswitch'' -ScriptBlock $__kswitchCompleterBlock' >> $PROFILE

# optionally use alias `s` instead of `kswitch`
echo "Set-Alias -Name s -Value kswitch" >> $PROFILE
echo 'Register-ArgumentCompleter -CommandName ''s'' -ScriptBlock $__kswitchCompleterBlock' >> $PROFILE

# re-source your profile
. $PROFILE
```

## Check that it works

Run `kswitch` (or `s` if you set the alias) from any terminal.
If the command is not found, open a new terminal or re-source your rc file (`.zshrc`, `.bashrc`, …).

That should display the contexts the tool can find with the default configuration.
If you get `Error: you need to point kswitch to a kubeconfig file` or do not see all desired contexts,
follow [kubeconfig stores](kubeconfig_stores.md) for configuration.
