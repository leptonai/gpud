#! /bin/sh

# based on https://tailscale.com/install.sh
# original copyright:
# Copyright (c) Tailscale Inc & AUTHORS
# SPDX-License-Identifier: BSD-3-Clause

set -eu

main() {
  OS=
  if type uname >/dev/null 2>&1; then
    case "$(uname)" in
    	Darwin)
  			OS="darwin"
  			echo "macos is not supported yet, follow https://github.com/leptonai/gpud/blob/main/docs/INSTALL-GPUD.md to build by yourself"
  			exit 1
  			;;
  		Linux)
  			OS="linux"
  			;;
  	  *)
  	    echo "OS $(uname) is not supported, follow https://github.com/leptonai/gpud/blob/main/docs/INSTALL-GPUD.md to build by yourself"
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
  			echo "arm64 is not supported yet, follow https://github.com/leptonai/gpud/blob/main/docs/INSTALL-GPUD.md to build by yourself"
   			exit 1
  			;;
   	  *)
  	    echo "Processor $(uname -m) is not supported, follow https://github.com/leptonai/gpud/blob/main/docs/INSTALL-GPUD.md to build by yourself"
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
  VERSION=$(curl -fsSL https://pkg.gpud.dev/"$TRACK"_latest.txt)

  FILENAME=gpud_"$VERSION"_"$OS"_"$ARCH".tgz
  if [ -e "$FILENAME" ]; then
    echo "file '$FILENAME' already exists"
    exit 1
  fi

  DIR=/tmp/gpud_"$VERSION"
  if [ -d "$DIR" ]; then
    echo "temporal directory $DIR already exists"
    exit 1
  fi

  DLPATH=/tmp/"$FILENAME"
  curl -fsSL https://pkg.gpud.dev/"$FILENAME" -o "$DLPATH"
  tar xzf "$DLPATH" -C /tmp

  $SUDO cp -f "$DIR"/gpud /usr/sbin

  echo "installed gpud version $VERSION"
  rm /tmp/"$FILENAME"
  rm -rf "$DIR"
}

main
