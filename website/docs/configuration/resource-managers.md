---
sidebar_position: 5
---

# Resource Managers

This section contains information on the configuration required by
the resource managers supported by CEEMS.

## SLURM

The [SLURM collector](../components/ceems-exporter.md#slurm-collector)
in the CEEMS exporter relies on the job accounting information
(like CPU time and memory usage) in the cgroups that SLURM creates for
each job to estimate the energy and emissions for a given job. However,
depending on the cgroups version and SLURM configuration, this accounting
information might not be available. The following section provides guidelines
on how to configure SLURM to ensure that this accounting information is
always available.

Starting from [SLURM 22.05](https://slurm.schedmd.com/archive/slurm-22.05.0/cgroups.html),
SLURM supports both cgroups v1 and v2. When using cgroups v1,
SLURM might not contain accounting information in the cgroups.

### cgroups v1

The following configuration enables the necessary cgroups controllers
and provides the accounting information for jobs when cgroups v1 is used.

As stated in the [cgroups docs of SLURM](https://slurm.schedmd.com/cgroup.conf.html),
the cgroups plugin can be controlled by the configuration in this file.
An [example config](https://slurm.schedmd.com/cgroup.conf.html#OPT_/etc/slurm/cgroup.conf)
is also provided, which serves as a good starting point.

Along with the `cgroups.conf` file, certain configuration parameters are
required in the `slurm.conf` file as well. This information is provided
in the [SLURM docs](https://slurm.schedmd.com/cgroup.conf.html#OPT_/etc/slurm/slurm.conf)
as well.

:::important[IMPORTANT]

Although `JobAcctGatherType=jobacct_gather/cgroup` is presented as an
_optional_ configuration parameter, it _must_ be used to get the accounting
information for CPU usage. Without this configuration parameter, the CPU
time of the job will not be available in the job's cgroups.

:::

Besides the above configuration,
[SelectTypeParameters](https://slurm.schedmd.com/slurm.conf.html#OPT_SelectTypeParameters)
must be configured to set the core or CPU and memory as consumable resources.
This is highlighted in the documentation of the
[ConstrainRAMSpace](https://slurm.schedmd.com/cgroup.conf.html#OPT_ConstrainRAMSpace)
configuration parameter in the [`cgroups.conf` docs](https://slurm.schedmd.com/cgroup.conf.html).

In conclusion, here are the necessary configuration excerpts:

```ini
# cgroups.conf

ConstrainCores=yes
ConstrainDevices=yes
ConstrainRAMSpace=yes
ConstrainSwapSpace=yes
```

```ini
# slurm.conf

ProctrackType=proctrack/cgroup
TaskPlugin=task/cgroup,task/affinity
JobAcctGatherType=jobacct_gather/cgroup 
SelectType=select/con_tres
SelectTypeParameters=CR_CPU_Memory # or CR_Core_Memory
AccountingStorageTRES=gres/gpu # or any other TRES resources declared in your SLURM config
```

### cgroups v2

For cgroups v2, SLURM should create the proper cgroups for every job without
any special configuration. However, the configuration presented for
[cgroups v1](#cgroups-v1) is applicable to cgroups v2, and it is advised to use
that configuration for cgroups v2 as well.

## Libvirt

The libvirt collector is meant to be used for OpenStack clusters. There is no
special configuration needed, as OpenStack will take care of configuring libvirt
and QEMU to enable all relevant cgroup controllers.

## Kubernetes

Unlike SLURM and Openstack, Kubernetes do not have a concept of user object. Any user
with a valid kubeconfig file can create k8s resources like Pod, Deployment, _etc_. Thus,
it is not possible to attribute a pod to a user natively in Kubernetes. Moreover, pod
spec only contains the namespace that the pod is created in and not the name of the
user who created that pod resource. This poses a significant challenge for CEEMS as
a definite pod to user association is needed to impose estimate the user's usage
statistics and strict access control.

### Admission Controller

In order to address above mentioned limitation of Kubernetes, CEEMS ships an admission
controller to add the name of the user to the resource spec as an annotation. By default,
the admission controller will add the name of the user who is creating that resource as
an annotation `ceems.io/created-by` to that resource. This annotation is eventually passed
down to the pod which is the most primitive unit of compute in Kubernetes context. For
instance, if a user creates a Deployment, the admission controller of the CEEMS will add
an annotation `ceems.io/created-by` with username as the value. However, it is the Deployment
controller service account which creates the pods defined in the Deployment spec. Instead
of adding the name of that controller service account, the admission controller ensures the
name of the user in `ceems.io/created-by` is passed down to the pod as well. Eventually,
CEEMS API server can be configured to search for a list of annotations to get the username
from.

:::important[IMPORTANT]

The admission controller shipped by CEEMS do not stop anyone from creating Kubernetes
resources. It only adds annotations to the resource specs in a mutation hook and
checks for the presence of the required annotation in validate hook. Even if the
required annotation is absent in the resource spec, it passes the hook with a log
entry for debugging purposes for the operators.

:::

For this approach to work, there should be a strict RBAC model that must be followed.
Namespaces must be used as abstraction for different projects and users must be confined
to different namespaces. Moreover, this needs that end users should create the Kubernetes
resources themselves instead of some other service for them. For example, let's take
examples of [JupyterHub](https://z2jh.jupyter.org/en/stable/) and
[Kubeflow](https://www.kubeflow.org/). In both these cases, end users creates Kubernetes
resources _via_ web UI interface where service accounts creates pod on behalf of users.
In these cases, Admission controller will add the name of the service account to the
annotation as retrieving the "real" username is not trivial.

#### JupyterHub

In the case of JupyterHub, there is a workaround using
[`extra_annotations`](https://jupyterhub-kubespawner.readthedocs.io/en/latest/spawner.html#kubespawner.KubeSpawner.extra_annotations)
configuration parameter supported by
[KubeSpawner](https://jupyterhub-kubespawner.readthedocs.io/en/latest/index.html).
By setting the following configuration parameter, each pod can be annotated with the
current username.

```python
c.KubeSpawner.extra_annotations = {'ceems.io/created-by': f'{username}'}
```

For a better isolation, each project must use its own namespace and single user
servers must use the project's namespace. This can be achieved in different ways
and we recommend to consult the
[KubeSpawner's API docs](https://jupyterhub-kubespawner.readthedocs.io/en/latest/spawner.html#).

#### Kubeflow

Kubeflow uses Kubernetes RBAC model to support
[multi-tenancy](https://www.kubeflow.org/docs/components/central-dash/profiles/).
This means each project will have a dedicated namespace and users will have roles
created to create resources in their project namespaces. Thus, the association
between the project namespaces and users can be retrieved by listing all the rolebindings
of Kubernetes cluster. This is what CEEMS API server does to maintain a list of users
and their namespaces. However, there is no easy way to add the username annotation to
the Kubernetes resources created by Kubeflow. The consequence of this limitation is that
CEEMS can only estimate usage metrics at the project level and not really user level.
Operators can encourage the users to add their username as a custom annotation to their
resource specs and configure the name of that custom annotation with CEEMS API server
to enable accounting of user statistics.

### CRDs

The admission controller that is shipped with CEEMS only works for default Kubernetes
resources and not with CRDs. Thus usage accounting and access control of CEEMS cannot
support CRDs like [Argo CD](https://argo-cd.readthedocs.io/en/stable/),
[Argo Workflows](https://argoproj.github.io/workflows/). A workaround is available in
this kind of scenario as well.

For example, depending on the cluster configuration, Argo CD allows to deploy
resources to arbitrary namespace. And Argo CD uses its own
[RBAC model](https://argo-cd.readthedocs.io/en/stable/operator-manual/rbac/) and
do not rely on Kubernetes RBAC model. Thus, it is not possible to get users and their
associated namespaces by simply listing rolebindings. The operators needs to maintain
this association in a different file and CEEMS API server is capable of reading the
usernames and their namespaces from this file. This is a very simple YAML file containing
name of the namespace as key and list of usernames as values. For instance, the a sample
file can be as follows:

```yaml
users:
  ns1:
    - usr1
    - usr2
  ns2:
    - usr2
    - usr3
```

Typically, in a Kubernetes cluster this information can be created as a configmap and
mounted on the CEEMS API server pod as a file. This allows CEEMS to account the usage
statistics per namespace and allow to impose access control.
