# litestream

[litestream](https://litestream.io/) streaming DB replication service for SQLite DBs.
It is an ideal solution to backup CEEMS API server DB in real time in asynchronous
manner that has minimal to no impact on CEEMS API DB performance. litestream supports
replicating DB to different types of storage like S3, Azure blob, Google Buckets, filesystem,
SFTP, _etc_. The sample config file presented here shows on how to replicate the DB to S3
and filesystem storage backends.

## Starting the service

litestream can be deployed in different ways which are explained in detail in [docs](https://litestream.io/guides/).
The service can be started as follows:

```bash
litestream replicate -config=/etc/litestream/config.yml
```

assuming the config file is installed at `/etc/litestream/config.yml`.

An [Ansible role](https://mahendrapaipuri.github.io/ansible/branch/main/litestream_role.html)
is also available to be able to install and configure litestream using Ansible.
