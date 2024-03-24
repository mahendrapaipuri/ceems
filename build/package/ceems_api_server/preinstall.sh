#!/bin/sh

uid=ceemsapi
gid=ceemsapi

# Create user and group if nonexistent
if [ ! $(getent group ${gid}) ]; then
   groupadd -r ${gid} > /dev/null 2>&1 || :
fi
if [ ! $(getent passwd ${uid}) ]; then
   useradd -M -r -d / -g ${gid} ${uid} > /dev/null 2>&1 || :
fi

# Create /var/lib/ceems_api_server directory and set ownership to ceemsapi user and root group
mkdir -p /var/lib/ceems_api_server
chown -R ${uid}:root /var/lib/ceems_api_server
chmod 0700 /var/lib/ceems_api_server
