---
title: Writing context to KUBECONFIG
---

# Writing context to KUBECONFIG

By default kswitch uses **terminal isolation**: each shell window gets its own temporary kubeconfig file in
`~/.kube/.switch_tmp/`, and the real `~/.kube/config` (or whatever `$KUBECONFIG` points to) is never
modified. This is ideal for multi-window workflows where every terminal targets a different cluster.

However, some tools cannot follow the `$KUBECONFIG` environment variable:

- IDEs (IntelliJ, VS Code with certain extensions) that read kubeconfig at startup
- Dashboard tools running outside your shell
- Scripts that call `kubectl` in a subprocess that does not inherit your shell environment
- New terminals that open with the default kubeconfig instead of your current selection

For these cases kswitch offers **write mode**, which merges the selected context into the real
kubeconfig file and sets `current-context` there.

## Using write mode

### One-off: CLI flag

```sh
kswitch --write      # long form
kswitch -w           # short form
```

After you pick a context in the TUI, kswitch merges it into the target file and sets `current-context`.
Your shell's `$KUBECONFIG` is also updated to point to the real file, so the current terminal stays in
sync.

### Persistent default: SwitchConfig

To make write mode the default for every switch, add `writeToKubeconfig: true` to your
`~/.kube/switch-config.yaml`:

```yaml
kind: SwitchConfig
version: v1alpha1
writeToKubeconfig: true
kubeconfigStores:
  - kind: filesystem
    paths:
      - ~/.kube/configs
```

The CLI flag still overrides: if the config says `true` and you want isolation just this once, there
is currently no inverse flag — simply omit `writeToKubeconfig` from the config and pass `-w` only
when needed.

## Target file resolution

| Situation | File written to |
|---|---|
| `$KUBECONFIG` is unset | `~/.kube/config` |
| `$KUBECONFIG` points to a kswitch tmp file (`~/.kube/.switch_tmp/…`) | `~/.kube/config` |
| `$KUBECONFIG` points to a real file (e.g. `/custom/path/config`) | that file |

## Merging behaviour

Write mode reuses the same merge logic as the [`merge-to-default-kubeconfig`](#merge-to-default-kubeconfig-command)
subcommand:

- **Clusters, contexts, and users** with the same name are replaced by the incoming version.
- Entries not present in the selected kubeconfig are left untouched.
- `current-context` is set to the context you selected.
- If the target file does not exist it is created with a minimal empty kubeconfig skeleton.

## `merge-to-default-kubeconfig` command

The `merge-to-default-kubeconfig` subcommand performs the same merge on demand, without going
through the interactive TUI. It reads whichever file `$KUBECONFIG` currently points to (your active
kswitch context) and merges it into the target file.

```sh
kswitch merge-to-default-kubeconfig
```

This is useful when you already switched context in the current shell and now want to "promote" that
choice to the shared kubeconfig so that other tools (IDE, new terminals) pick it up.

**Example workflow:**

```sh
# Switch context in the current shell (normal isolation mode)
kswitch

# Promote the active context to ~/.kube/config so IntelliJ picks it up
kswitch merge-to-default-kubeconfig
```

Output:

```
✅ Merged context my-cluster into /Users/you/.kube/config
```

## Comparison: isolation vs write mode

| | Default (isolation) | Write mode (`-w`) |
|---|---|---|
| Current terminal | ✅ Sees selected context | ✅ Sees selected context |
| Other open terminals | Unchanged | Unchanged |
| New terminals | Use default kubeconfig | Use updated `~/.kube/config` |
| IDEs / non-shell tools | Unchanged | ✅ Pick up the new `current-context` |
| `~/.kube/config` modified | ❌ Never | ✅ Yes |
| Multiple clusters at once | ✅ One per terminal | ❌ Shared `current-context` |
