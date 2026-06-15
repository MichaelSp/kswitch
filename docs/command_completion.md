# Command completion 

**Note**: this is typically not needed, as when installing the shell function manually via [source the shell function](#source-the-shell-function), the completion script is already included.

Install the completion script by running:

### Bash

```sh
echo 'source <(switch completion bash)' >> ~/.bashrc
```
### Zsh
```sh
echo 'source <(switch completion zsh)' >> ~/.zshrc
```
### Fish
```sh
echo 'kswitch completion fish | source' >> ~/.config/fish/config.fish
```

### Powershell
```powershell
echo 'kswitch completion powershell' >> $PROFILE
echo 'Register-ArgumentCompleter -CommandName ''kswitch_windows_amd64'' -ScriptBlock $__kswitchCompleterBlock' >> $PROFILE
echo 'Register-ArgumentCompleter -CommandName ''kswitch'' -ScriptBlock $__kswitchCompleterBlock' >> $PROFILE
. $PROFILE
```
