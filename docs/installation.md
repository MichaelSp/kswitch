# Installation

The kswitch installation consists of both a `kswitch` binary and a shell script which needs to be sourced.

**NOTE**: to invoke kswitch, do not call the `kswitch` binary directly from the command line. 
Instead, use the sourced shell function as described in [source the shell function](#required-source-the-shell-function).

## Option 1 - Homebrew
**NOTE**: `fish` users please follow [install via Github releases](#option-2---github-releases) as the shell script only works for `zsh` and `bash` shells.

Install the `kswitch` binary with `homebrew`.
```
brew install MichaelSp/switch/switch
```

Next, follow [required: source the shell function](#required-source-the-shell-function).

### Option 2 - MacPorts
**NOTE**: `fish` users please follow [install via Github releases](#option-2---github-releases) as the shell script only works for `zsh` and `bash` shells.

Mac users can also install both `switch.sh` and `kswitch` from [MacPorts](https://www.macports.org)
```
sudo port selfupdate
sudo port install kswitch
```

Next, follow [required: source the shell function](#required-source-the-shell-function).

### Option 2 - Github releases

Download the kswitch binary
```sh
OS=linux                        # Pick the right os: linux, darwin (intel only)
VERSION=0.9.3                   # Pick the current version.

curl -L -o /usr/local/bin/kswitch https://github.com/danielfoehrKn/kswitch/releases/download/${VERSION}/kswitch_${OS}_amd64
chmod +x /usr/local/bin/kswitch
```
If you are using Windows, go to the release webpage using you browser and download the windows binary: <https://github.com/danielfoehrKn/kswitch/releases/>\
Then copy it to a folder available in your path. To add a folder to your path, you can use the ``Environment Variables`` tool for the Windows' PowerToys: <https://learn.microsoft.com/en-us/windows/powertoys/environment-variables>\
If you need to add a folder to the path for the current powershell session, you can run ``$env:Path += ';C:\myfolder'``



Next, follow [required: source the shell function](#required-source-the-shell-function).

### Option 3 - From source

```
go get github.com/MichaelSp/kswitch
```

From the repository root run `make build-kswitch`.
This builds the binaries to `/hack/switch/`.
Copy the build binary for your OS/Architecture to e.g. `/usr/local/bin`.

Next, follow [required: source the shell function](#required-source-the-shell-function).

## Required: Source the shell function

Source the shell function which is used to call the `kswitch` binary. 
For `zsh/bash` the name of the shell function is `switch` and for `fish` its `kswitch`.
Additionally, installs the command completion script.

### Bash

```sh
echo 'source <(kswitch init bash)' >> ~/.bashrc

# optionally use alias `s` instead of `switch`
echo 'alias s=switch' >> ~/.bashrc
echo 'complete -o default -F __start_kswitch s' >> ~/.bashrc

# optionally use `kswitch` as an alias for `switch`
# NOTE: do NOT alias `kswitch` to the raw binary — it must go through the shell wrapper
# so that KUBECONFIG is exported into your current shell session.
echo 'alias kswitch=switch' >> ~/.bashrc
```
### Zsh
```sh
echo 'source <(kswitch init zsh)' >> ~/.zshrc

# optionally use alias `s` instead of `switch`
echo 'alias s=switch' >> ~/.zshrc

# optionally use `kswitch` as an alias for `switch`
# NOTE: do NOT alias `kswitch` to the raw binary — it must go through the shell wrapper
# so that KUBECONFIG is exported into your current shell session.
echo 'alias kswitch=switch' >> ~/.zshrc

# optionally use command completion
echo 'source <(switch completion zsh)' >> ~/.zshrc
```
### Fish
Fish shell have a built-in `switch` function. Hence, differently from `zsh` shells, the kswitch function is called `kswitch`.
```sh
echo 'kswitch init fish | source' >> ~/.config/fish/config.fish

# optionally use alias `s` instead of `kswitch` (add to config.fish)
function s --wraps kswitch
        kswitch $argv;
end
```
### Powershell
Powershell shell have a built-in `switch` function. Hence, differently from `zsh` shells, the kswitch function is called `kswitch`.

```powershell
kswitch_windows_amd64.exe init powershell >> $PROFILE

# add this for the autocomplete to work
echo 'Register-ArgumentCompleter -CommandName ''kswitch_windows_amd64'' -ScriptBlock $__kswitchCompleterBlock' >> $PROFILE
echo 'Register-ArgumentCompleter -CommandName ''kswitch'' -ScriptBlock $__kswitchCompleterBlock' >> $PROFILE

# optionally use alias `s` instead of `kswitch` (add to $PROFILE)
echo "" >> $PROFILE
echo "Set-Alias -Name s -Value kswitch" >> $PROFILE
echo 'Register-ArgumentCompleter -CommandName ''s'' -ScriptBlock $__kswitchCompleterBlock' >> $PROFILE

# source your profile again
. $PROFILE
```

## Check that it works

If you installed kswitch correctly, you can run the command `switch` (zsh, bash) or `kswitch` (fish, powershell) or alternatively the alias `s` from the terminal.
In case the terminal can't find the function, you might need to open another terminal or re-source your config file (`.zshrc`,`.bashrc`,...).

That should display the contexts the tool can find with the default configuration.
If you get the error `Error: you need to point kswitch to a kubeconfig file` or do not see all
desired kubeconfig contexts that you want to choose from, follow
[kubeconfig stores](kubeconfig_stores.md) for the configuration.
