#!/bin/sh

uid=ceems
gid=ceems

# Create user and group if nonexistent
if [ ! $(getent group ${gid}) ]; then
   groupadd -r ${gid} > /dev/null 2>&1 || :
fi
if [ ! $(getent passwd ${uid}) ]; then
   useradd -M -r -d / -g ${gid} ${uid} > /dev/null 2>&1 || :
fi

# Create /var/lib/ceems directory and set ownership to ceems user and root group
mkdir -p /var/lib/ceems
chown -R ${uid}:root /var/lib/ceems
chmod 0700 /var/lib/ceems
