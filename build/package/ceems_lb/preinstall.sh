#!/bin/sh

uid=ceemslb
gid=ceemslb

# Create user and group if nonexistent
if [ ! $(getent group ${gid}) ]; then
   groupadd -r ${gid} > /dev/null 2>&1 || :
fi
if [ ! $(getent passwd ${uid}) ]; then
   useradd -M -r -d / -g ${gid} ${uid} > /dev/null 2>&1 || :
fi
