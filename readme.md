# cl

cl is a simple tool that allows you to run multiple commands concurrently
across a cluster of servers via SSH. It works by taking the name of the cluster
to run the command across, followed by the command to run like so.

```
$ cl uat systemctl status postgresql
```

cl reads expects a `ClFile` to be in the current directory from where the
command is invoked. This is a plain-text file that describes the cluster, and
how they can be accessed via SSH.

```
uat:
  db@db-host-1 ~/.ssh/id_rsa
  db@db-host-2 ~/.ssh/id_rsa
  db@db-host-3 ~/.ssh/id_rsa
```

## The ClFile

The `ClFile` expects the cluster of servers to be organised in the below format,
where the heading is the name that will be used via the command, and each
subsequent entry is the server to connect to.

```
[name]:
  [user]@[host]:[port] [identity]
```

**[name]:**

A human readable string that specifies the alias for the cluster machines.


**[user]**

The user to connect to the machine as, if not specified then `$USER` will be
used instead.

**[port]**

The port to connect to on the machine, if not specified then `22` will be used
instead.

**[identity]**

The identity file to use during SSH authentication, if not specified then
`~/.ssh/id_rsa` will be used by default.

There is no limit to the number of clusters that can be specifed in the `ClFile`.

```
uat:
  db@db-host-1 ~/.ssh/id_rsa
  db@db-host-2 ~/.ssh/id_rsa
  db@db-host-3 ~/.ssh/id_rsa

prod:
  db@prod-db-host1:1234 ~/.ssh/id_prod_db
  prod-db-host2:44
  prod-db-host3 ~/.ssh/id_rsa
```
