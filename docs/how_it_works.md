---
title: How it works
---

# How it works

`Kswitch` consists of two components:

`switch.sh` - contains the shell function `switch()` which is the entry point to Kswitch.
`kswitch`  - a go binary which is executed by the switch shell function and handles context selection and manipulation of the selected Kubeconfig file.

For a proper installation
 - you have to source the `switch.sh` script
 - make the kswitch binary available in your $PATH (the switch.sh shell script looks for the kswitch binary on the $PATH).

**Flow**

1) User executes `$ switch` (bash function available via the sourced **switch.sh** script) 
2) The `switch.sh` script discovers and executes the kswitch binary from the $PATH
3) The `kswitch` binary searches for kubeconfigs in the [kubeconfig stores](kubeconfig_stores.md) configured in the `SwitchConfig` configuration file.
4) The `kswitch` binary displays a fuzzy search for kubeconfig context names
5) The user selects on context name
6) The `kswitch` binary creates a temporary copy of the selected kubeconfig file, sets the `current-context` and writes the kubeconfig to `~/.kube/switch_tmp`
7) The `kswitch` binary writes the filepath to the kubeconfig to STDOUT
8) The `switch.sh` script captures this filepath and executes `export KUBECONFIG=</path/to/tmp/kubeconfig/file>` 

Each terminal window operates on its own copy of the kubeconfig file (terminal window isolation).
