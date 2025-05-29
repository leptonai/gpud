#! /bin/sh

# based on https://tailscale.com/install.sh
# original copyright:
# Copyright (c) Tailscale Inc & AUTHORS
# SPDX-License-Identifier: BSD-3-Clause

set -e

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
  # ref. https://fedoraproject.org/wiki/Changes/Unify_bin_and_sbin
  $SUDO cp -f "$DIR"/gpud /usr/local/bin

  echo "installed gpud version $APP_VERSION"
  rm /tmp/"$FILENAME"
  rm -rf "$DIR"
}

main "$@"
