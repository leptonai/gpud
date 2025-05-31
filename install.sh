#! /bin/sh

# based on https://tailscale.com/install.sh
# original copyright:
# Copyright (c) Tailscale Inc & AUTHORS
# SPDX-License-Identifier: BSD-3-Clause

set -e

# Function to compare semantic versions
# Returns: -1 if version1 < version2, 0 if equal, 1 if version1 > version2
version_compare() {
  local version1="$1"
  local version2="$2"
  
  # Remove 'v' prefix if present
  version1=$(echo "$version1" | sed 's/^v//')
  version2=$(echo "$version2" | sed 's/^v//')
  
  # Handle release candidates
  local v1_base v1_rc v2_base v2_rc
  if echo "$version1" | grep -q -- "-rc-"; then
    v1_base=$(echo "$version1" | cut -d'-' -f1)
    v1_rc=$(echo "$version1" | cut -d'-' -f3)
  else
    v1_base="$version1"
    v1_rc="9999"  # Treat non-rc as higher than any rc
  fi
  
  if echo "$version2" | grep -q -- "-rc-"; then
    v2_base=$(echo "$version2" | cut -d'-' -f1)
    v2_rc=$(echo "$version2" | cut -d'-' -f3)
  else
    v2_base="$version2"
    v2_rc="9999"  # Treat non-rc as higher than any rc
  fi
  
  # Compare base versions using sort -V (version sort)
  local base_cmp
  if [ "$v1_base" = "$v2_base" ]; then
    base_cmp="0"
  elif printf '%s\n%s\n' "$v1_base" "$v2_base" | sort -V | head -n1 | grep -q "^$v1_base$"; then
    base_cmp="-1"  # v1_base < v2_base
  else
    base_cmp="1"   # v1_base > v2_base
  fi
  
  # If base versions are equal, compare RC numbers
  if [ "$base_cmp" = "0" ]; then
    if [ "$v1_rc" -lt "$v2_rc" ]; then
      echo "-1"
    elif [ "$v1_rc" -gt "$v2_rc" ]; then
      echo "1"
    else
      echo "0"
    fi
  else
    echo "$base_cmp"
  fi
}

main() {
  OS=
  if type uname >/dev/null 2>&1; then
    case "$(uname)" in
    	Darwin)
  			OS="darwin"
  			echo "macos is not supported yet, follow https://github.com/leptonai/gpud/blob/main/docs/INSTALL.md to build by yourself"
  			exit 1
  			;;
  		Linux)
  			OS="linux"
  			;;
  	  *)
  	    echo "OS $(uname) is not supported, follow https://github.com/leptonai/gpud/blob/main/docs/INSTALL.md to build by yourself"
  	    exit 1
  	    ;;
  	esac
  fi

  ARCH=
  if type uname >/dev/null 2>&1; then
  	case "$(uname -m)" in
  		x86_64)
    		ARCH="amd64"
  			;;
  		arm64|aarch64)
  			ARCH="arm64"
  			;;
   	  *)
  	    echo "Processor $(uname -m) is not supported, follow https://github.com/leptonai/gpud/blob/main/docs/INSTALL.md to build by yourself"
  	    exit 1
  	    ;;
  	esac
  fi

  CAN_ROOT=
  SUDO=
  if [ "$(id -u)" = 0 ]; then
    CAN_ROOT=1
    SUDO=""
  elif type sudo >/dev/null; then
    CAN_ROOT=1
    SUDO="sudo"
  elif type doas >/dev/null; then
    CAN_ROOT=1
    SUDO="doas"
  fi

  if [ "$CAN_ROOT" != "1" ]; then
    echo "This installer needs to run commands as root."
    echo "We tried looking for 'sudo' and 'doas', but couldn't find them."
    echo "Either re-run this script as root, or set up sudo/doas."
    exit 1
  fi

  TRACK="${TRACK:-unstable}"
  if [ -n "$1" ]; then
    APP_VERSION="$1"
  else
    # e.g., https://pkg.gpud.dev/unstable_latest.txt
    APP_VERSION=$(curl -fsSL https://pkg.gpud.dev/"$TRACK"_latest.txt)
  fi

  if ! type lsb_release >/dev/null 2>&1; then
    . /etc/os-release
    OS_NAME=$(echo "$ID" | tr '[:upper:]' '[:lower:]')
    OS_VERSION=$(echo "$APP_VERSION" | tr -d '"')
  else
    # e.g., ubuntu22.04, ubuntu24.04
    OS_NAME=$(lsb_release -i -s | tr '[:upper:]' '[:lower:]' 2>/dev/null)
    OS_VERSION=$(lsb_release -r -s 2>/dev/null || echo "")
  fi

  if [ "$OS_NAME" = "ubuntu" ]; then
    case "$OS_VERSION" in
      22.04|24.04)
        OS_DISTRO="_${OS_NAME}${OS_VERSION}"
        ;;
      *)
        echo "Ubuntu version $OS_VERSION is not supported, only 22.04, and 24.04 are supported."
        exit 1
        ;;
    esac
  elif [ "$OS_NAME" = "amzn" ]; then
    case "$OS_VERSION" in
      2|2023)
        OS_DISTRO="_${OS_NAME}${OS_VERSION}"
        ;;
      *)
        echo "Amazon Linux version $OS_VERSION is not supported, only version 2, 2023 is supported."
        exit 1
        ;;
    esac
  else
    OS_DISTRO=""
  fi

  FILEBASE=gpud_"$APP_VERSION"_"$OS"_"$ARCH""$OS_DISTRO"
  FILENAME=$FILEBASE.tgz
  if [ -e "$FILENAME" ]; then
    echo "file '$FILENAME' already exists"
    exit 1
  fi

  # same release always same contents
  # thus safe to remove in case of previous installation failure
  DIR=/tmp/$FILEBASE
  rm -rf "$DIR"

  mkdir "$DIR"
  DLPATH=/tmp/"$FILENAME"

  if ! curl -fsSL "https://pkg.gpud.dev/$FILENAME" -o "$DLPATH" 2>/tmp/gpud_curl_error.log; then
    echo "failed to download file from 'https://pkg.gpud.dev/$FILENAME'"

    echo "\nerror message:"
    cat /tmp/gpud_curl_error.log

    rm -f "$DLPATH" /tmp/gpud_curl_error.log
    exit 1
  fi
  rm -f /tmp/gpud_curl_error.log

  tar xzf "$DLPATH" -C "$DIR"

  # some os distros have "/usr/sbin" as read-only
  # we want to use "/usr/local/bin" instead
  # ref. https://fedoraproject.org/wiki/Changes/Unify_bin_and_sbin
  BIN_PATH="/usr/local/bin"

  # only "$APP_VERSION" after >= v0.5.0-rc-34, >= v0.5 supports "/usr/local/bin/gpud"
  # "v0.5.0-rc-33" does not support "/usr/local/bin/gpud", thus we need to use "/usr/sbin/gpud"
  # ref. https://github.com/leptonai/gpud/pull/846

  # check if version supports /usr/local/bin
  # Supported: >= v0.5.0-rc-34
  if [ "$(version_compare "$APP_VERSION" "0.5.0-rc-34")" -ge 0 ]; then
    BIN_PATH="/usr/local/bin"
  else
    BIN_PATH="/usr/sbin"
  fi
  $SUDO cp -f "$DIR"/gpud $BIN_PATH

  echo "installed gpud version $APP_VERSION in the path $BIN_PATH/gpud"
  rm /tmp/"$FILENAME"
  rm -rf "$DIR"
}

main "$@"
