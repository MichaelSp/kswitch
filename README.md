<p align="center">
  <a href="https://MichaelSp.github.io/kswitch">
    <img src="https://raw.githubusercontent.com/MichaelSp/kswitch/main/website/src/assets/banner.png" alt="kswitch banner" width="900"/>
  </a>
</p>

# kswitch

![Latest GitHub release](https://img.shields.io/github/v/release/MichaelSp/kswitch.svg)
[![Build](https://github.com/MichaelSp/kswitch/workflows/Build/badge.svg)](https://github.com/MichaelSp/kswitch/actions?query=workflow%3A"Build")
[![Go Report Card](https://goreportcard.com/badge/github.com/MichaelSp/kswitch)](https://goreportcard.com/badge/github.com/MichaelSp/kswitch)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

**The kubectx for operators.** Fuzzy search across 14+ cloud providers, per-terminal context isolation, history navigation, and extensible hooks — built for large-scale Kubernetes.

## Highlights

- **Unified fuzzy search** — one search over EKS, AKS, GKE, Gardener, Vault, filesystem, and more
- **Terminal isolation** — each window targets a different cluster; the original kubeconfig is never modified
- **Write mode** (`-w`) — merge the selected context into the real kubeconfig for IDE and cross-terminal visibility
- **History** — every `{context, namespace}` tuple recorded; jump back with `switch .` or `switch -`
- **Context aliases** — human-friendly names for cryptic generated context names
- **Search index cache** — instant results across massive directories or slow remote stores
- **Hooks** — run arbitrary executables before search to sync, refresh, or rotate credentials
- **Drop-in replacement** for `kubectx` — set `alias kubectx=switch` and keep your workflow

## Demo

<p align="center">
  <img src="resources/gifs/switch-demo-large.gif" alt="kswitch context switch demo" width="48%"/>
  <img src="resources/gifs/namespace.gif" alt="kswitch namespace switch demo" width="48%"/>
</p>

## Install

### macOS

**Step 1 — Install the binary** (pick one):

```sh
# Homebrew (recommended)
brew tap MichaelSp/kswitch
brew trust --cask michaelsp/kswitch/kubectl-switch
brew install kubectl-switch
```

```sh
# Direct download
curl -L -o /usr/local/bin/kubectl-switch \
  https://github.com/MichaelSp/kswitch/releases/latest/download/kubectl-switch_darwin_amd64
chmod +x /usr/local/bin/kubectl-switch
```

**Step 2 — Wire up the shell function** (pick one):

```sh
# zsh
echo 'source <(kubectl-switch init zsh)' >> ~/.zshrc && source ~/.zshrc
```

```sh
# bash
echo 'source <(kubectl-switch init bash)' >> ~/.bashrc && source ~/.bashrc
```

### Linux

**Step 1 — Install the binary:**

```sh
curl -L -o /usr/local/bin/kubectl-switch \
  https://github.com/MichaelSp/kswitch/releases/latest/download/kubectl-switch_linux_amd64
chmod +x /usr/local/bin/kubectl-switch
```

**Step 2 — Wire up the shell function** (pick one):

```sh
# bash
echo 'source <(kubectl-switch init bash)' >> ~/.bashrc && source ~/.bashrc
```

```sh
# zsh
echo 'source <(kubectl-switch init zsh)' >> ~/.zshrc && source ~/.zshrc
```

### Windows

**Step 1 — Install the binary:** Download `kubectl-switch_windows_amd64.exe` from the [releases page](https://github.com/MichaelSp/kswitch/releases/latest), rename it to `kubectl-switch.exe`, and place it in your `PATH`.

**Step 2 — Wire up the shell function:**

```powershell
kubectl-switch init powershell >> $PROFILE
. $PROFILE
```

Then type `switch` (bash/zsh) or `kubectl-switch` (fish/PowerShell) to start.

## Documentation

Full documentation is available at **[MichaelSp.github.io/kswitch](https://MichaelSp.github.io/kswitch)**:

- [Installation guide](https://MichaelSp.github.io/kswitch/installation/) — shell completion, all platforms
- [Kubeconfig stores](https://MichaelSp.github.io/kswitch/kubeconfig_stores/) — multi-provider setup
- [Search index](https://MichaelSp.github.io/kswitch/search_index/) — caching for large setups
- [Writing context to KUBECONFIG](https://MichaelSp.github.io/kswitch/write_to_kubeconfig/) — write mode and merge-to-default
- [Hooks](https://MichaelSp.github.io/kswitch/hooks/) — extensibility
- [Cloud provider guides](https://MichaelSp.github.io/kswitch/stores/eks/eks/) — EKS, AKS, GKE, Gardener, Vault, and more
