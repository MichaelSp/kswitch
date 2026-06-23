#!/usr/bin/env bash
# PLEASE KEEP IN SYNC WITH THE COMMAND `kubectl-switch init zsh/bash`
# REQUIRED FOR THE DEFAULT HOMEBREW INSTALLATION for zsh and bash

has_prefix() { case $2 in "$1"*) true;; *) false;; esac; }

function kswitch(){
#  if the executable path is not set, the kubectl-switch binary has to be on the path
# this is the case when installing it via homebrew

  local DEFAULT_EXECUTABLE_PATH="kubectl-switch"
  declare -a opts

  while test $# -gt 0; do
    case "$1" in
    --executable-path)
        EXECUTABLE_PATH="$2"
        shift
        ;;
    *)
        opts+=( "$1" )
        ;;
    esac
    shift
  done

  if [ -z "$EXECUTABLE_PATH" ]; then
    EXECUTABLE_PATH="$DEFAULT_EXECUTABLE_PATH"
  fi

  RESPONSE="$(KSWITCH_SHELL_WRAPPER=1 $EXECUTABLE_PATH "${opts[@]}")"
  if [ $? -ne 0 -o -z "$RESPONSE" ]; then
    printf "%s\n" "$RESPONSE"
    return $?
  fi

  # kubectl-switch returns a response that contains a kubeconfig path with a prefix "__ " to be able to
  # distinguish it from other responses which just need to write to STDOUT
  prefix="__ "
  if ! has_prefix "$prefix" "$RESPONSE" ; then
      printf "%s\n" "$RESPONSE"
      return
  fi

  # remove prefix
  RESPONSE=${RESPONSE#"$prefix"}

  #the response from the kubectl-switch binary is "kubeconfig_path,selected_context"
  remainder="$RESPONSE"
  KUBECONFIG_PATH="${remainder%%,*}"; remainder="${remainder#*,}"
  SELECTED_CONTEXT="${remainder%%,*}"; remainder="${remainder#*,}"

  if [ -z ${KUBECONFIG_PATH+x} ]; then
    # KUBECONFIG_PATH is not set
    printf "%s\n" "$RESPONSE"
    return
  fi

  if [ -z ${SELECTED_CONTEXT+x} ]; then
    # SELECTED_CONTEXT is not set
    printf "%s\n" "$RESPONSE"
    return
  fi

  # cleanup old temporary kubeconfig file
  local switchTmpDirectory="$HOME/.kube/.switch_tmp/config"
  if [[ -n "$KUBECONFIG" && "$KUBECONFIG" == *"$switchTmpDirectory"* ]]
  then
    \rm -f "$KUBECONFIG"
  fi

  export KUBECONFIG="$KUBECONFIG_PATH"
  printf "switched to context %s\n" "$SELECTED_CONTEXT"
}
