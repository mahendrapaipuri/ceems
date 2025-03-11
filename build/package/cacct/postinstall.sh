#!/bin/sh

# Step 1, get systemd version
systemd_version=$(systemctl --version | sed -nE "s/systemd ([0-9]+).*/\1/p")

cleanInstall() {
    printf "\033[32m Post Install of an clean install\033[0m\n"
    # Step 3 (clean install), enable the service in the proper way for this platform
    # rhel/centos7 cannot use ExecStartPre=+ to specify the pre start should be run as root
    # even if you want your service to run as non root.
    # noop
}

upgrade() {
    printf "\033[32m Post Install of an upgrade\033[0m\n"
    # Step 3(upgrade), do what you need
    # noop
}

# Step 2, check if this is a clean install or an upgrade
action="$1"
if  [ "$1" = "configure" ] && [ -z "$2" ]; then
    # Alpine linux does not pass args, and deb passes $1=configure
    action="install"
elif [ "$1" = "configure" ] && [ -n "$2" ]; then
    # deb passes $1=configure $2=<current version>
    action="upgrade"
fi

case "$action" in
  "1" | "install")
    cleanInstall
    ;;
  "2" | "upgrade")
    cleanInstall
    ;;
  *)
    # $1 == version being installed
    printf "\033[32m Alpine\033[0m"
    cleanInstall
    ;;
esac
