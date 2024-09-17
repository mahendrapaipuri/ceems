/* SPDX-License-Identifier: (GPL-3.0-only) */
// Nicked a lot of utility functions from https://github.com/cilium/tetragon project

#include "bpf_helpers.h"
#include "config.h"

#define NULL ((void *)0)

#ifndef CGROUP_SUPER_MAGIC
#define CGROUP_SUPER_MAGIC 0x27e0eb /* Cgroupv1 pseudo FS */
#endif

#ifndef CGROUP2_SUPER_MAGIC
#define CGROUP2_SUPER_MAGIC 0x63677270 /* Cgroupv2 pseudo FS */
#endif

/* Msg flags */
#define EVENT_ERROR_CGROUP_NAME	      0x010000
#define EVENT_ERROR_CGROUP_KN	      0x020000
#define EVENT_ERROR_CGROUP_SUBSYSCGRP 0x040000
#define EVENT_ERROR_CGROUP_SUBSYS     0x080000
#define EVENT_ERROR_CGROUPS	      0x100000
#define EVENT_ERROR_CGROUP_ID	      0x200000

/* Represent old kernfs node with the kernfs_node_id
 * union to read the id in 5.4 kernels and older
 */
struct kernfs_node___old {
	union kernfs_node_id id;
};

/**
 * __get_cgroup_kn() Returns the kernfs_node of the cgroup
 * @cgrp: target cgroup
 *
 * Returns the kernfs_node of the cgroup on success, NULL on failures.
 */
FUNC_INLINE struct kernfs_node *__get_cgroup_kn(const struct cgroup *cgrp)
{
	struct kernfs_node *kn = NULL;

	if (cgrp)
		bpf_probe_read(&kn, sizeof(cgrp->kn), _(&cgrp->kn));

	return kn;
}

/**
 * get_cgroup_kn_id() Returns the kernfs node id
 * @kernfs_node: target kernfs node
 *
 * Returns the kernfs node id on success, zero on failures.
 */
FUNC_INLINE __u64 __get_cgroup_kn_id(const struct kernfs_node *kn)
{
	__u64 id = 0;

	if (!kn)
		return id;

	/* Kernels prior to 5.5 have the kernfs_node_id, but distros (RHEL)
	 * seem to have kernfs_node_id defined for UAPI reasons even though
	 * its not used here directly. To resolve this walk struct for id.id
	 */
	if (bpf_core_field_exists(((struct kernfs_node___old *)0)->id.id)) {
		struct kernfs_node___old *old_kn;

		old_kn = (void *)kn;
		if (BPF_CORE_READ_INTO(&id, old_kn, id.id) != 0)
			return 0;
	} else {
		bpf_probe_read(&id, sizeof(id), _(&kn->id));
	}

	return id;
}

/**
 * get_cgroup_id() Returns cgroup id
 * @cgrp: target cgroup
 *
 * Returns the cgroup id of the target cgroup on success, zero on failures.
 */
FUNC_INLINE __u64 get_cgroup_id(const struct cgroup *cgrp)
{
	struct kernfs_node *kn;

	kn = __get_cgroup_kn(cgrp);
	return __get_cgroup_kn_id(kn);
}

/**
 * get_task_cgroup() Returns the accurate or desired cgroup of the css of
 * current task that we want to operate on.
 * @task: must be current task.
 * @subsys_idx: index of the desired cgroup_subsys_state part of css_set.
 * Passing a zero as a subsys_idx is fine assuming you want that.
 * @error_flags: error flags that will be ORed to indicate errors on
 * failures.
 *
 * Returns the cgroup of the css part of css_set of current task and is
 * indexed at subsys_idx on success. NULL on failures, and the error_flags
 * will be ORed to indicate the corresponding error.
 *
 * To get cgroup and kernfs node information we want to operate on the right
 * cgroup hierarchy which is setup by user space. However due to the
 * incompatibility between cgroup v1 and v2; how user space initialize and
 * install cgroup controllers, etc, it can be difficult.
 *
 * Use this helper and pass the css index that you consider accurate and
 * which can be discovered at runtime in user space.
 * Usually it is the 'memory' or 'pids' indexes by reading /proc/cgroups
 * file where each line number is the index starting from zero without
 * counting first comment line.
 */
FUNC_INLINE struct cgroup *
get_task_cgroup(struct task_struct *task, __u32 subsys_idx, __u32 *error_flags)
{
	struct cgroup_subsys_state *subsys;
	struct css_set *cgroups;
	struct cgroup *cgrp = NULL;

	bpf_probe_read(&cgroups, sizeof(cgroups), _(&task->cgroups));
	if (unlikely(!cgroups)) {
		*error_flags |= EVENT_ERROR_CGROUPS;
		return cgrp;
	}

	/* We are interested only in the cpuset, memory or pids controllers
	 * which are indexed at 0, 4 and 11 respectively assuming all controllers
	 * are compiled in.
	 * When we use the controllers indexes we will first discover these indexes
	 * dynamically in user space which will work on all setups from reading
	 * file: /proc/cgroups. If we fail to discover the indexes then passing
	 * a default index zero should be fine assuming we also want that.
	 *
	 * Reference: https://elixir.bootlin.com/linux/v5.19/source/include/linux/cgroup_subsys.h
	 *
	 * Notes:
	 * Newer controllers should be appended at the end. controllers
	 * that are not upstreamed may mess the calculation here
	 * especially if they happen to be before the desired subsys_idx,
	 * we fail.
	 */
	if (unlikely(subsys_idx > pids_cgrp_id)) {
		*error_flags |= EVENT_ERROR_CGROUP_SUBSYS;
		return cgrp;
	}

	/* Read css from the passed subsys index to ensure that we operate
	 * on the desired controller. This allows user space to be flexible
	 * and chose the right per cgroup subsystem to use in order to
	 * support as much as workload as possible. It also reduces errors
	 * in a significant way.
	 */
	bpf_probe_read(&subsys, sizeof(subsys), _(&cgroups->subsys[subsys_idx]));
	if (unlikely(!subsys)) {
		*error_flags |= EVENT_ERROR_CGROUP_SUBSYS;
		return cgrp;
	}

	bpf_probe_read(&cgrp, sizeof(cgrp), _(&subsys->cgroup));
	if (!cgrp)
		*error_flags |= EVENT_ERROR_CGROUP_SUBSYSCGRP;

	return cgrp;
}

/**
 * ceems_get_current_cgroupv1_id() Returns the accurate cgroup id of current task running
 * under cgroups v1.
 *
 * Returns the cgroup id of current task on success, zero on failures.
 */
FUNC_INLINE __u64 ceems_get_current_cgroupv1_id(int subsys_idx)
{
	__u32 error_flags;
	struct cgroup *cgrp;
	struct task_struct *task;

	task = (struct task_struct *)bpf_get_current_task();

	// NB: error_flags are ignored for now
	cgrp = get_task_cgroup(task, subsys_idx, &error_flags);
	if (!cgrp)
		return 0;

	return get_cgroup_id(cgrp);
}

/**
 * ceems_get_current_cgroup_id() Returns the accurate cgroup id of current task.
 *
 * Returns the cgroup id of current task on success, zero on failures.
 */
FUNC_INLINE __u64 ceems_get_current_cgroup_id(void)
{
	__u64 cgrpfs_magic = 0;
	struct conf *cfg;
	int zero = 0, subsys_idx = 1;

	cfg = bpf_map_lookup_elem(&conf_map, &zero);
	if (cfg) {
		/* Select which cgroup version */
		cgrpfs_magic = cfg->cgrp_fs_magic;
		/* Select which cgroup subsystem */
		subsys_idx = cfg->cgrp_subsys_idx;
	}

	/*
	 * Try the bpf helper on the default hierarchy if available
	 * and if we are running in unified cgroupv2
	 */
	if (cgrpfs_magic == CGROUP2_SUPER_MAGIC) {
		return bpf_get_current_cgroup_id();
	}

	return ceems_get_current_cgroupv1_id(subsys_idx);
}
