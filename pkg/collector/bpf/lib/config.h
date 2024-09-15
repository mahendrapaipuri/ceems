/* SPDX-License-Identifier: (GPL-3.0-only) */

#ifndef __CONF_
#define __CONF_

/* Runtime configuration */
struct conf {
	__u64 cgrp_subsys_idx; /* Tracked cgroup subsystem state index at compile time */
	__u64 cgrp_fs_magic; /* Cgroupv1 or Cgroupv2 */
};

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, struct conf);
} conf_map SEC(".maps");

#endif // __CONF_
