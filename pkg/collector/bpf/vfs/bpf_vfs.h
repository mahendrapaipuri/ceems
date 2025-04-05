//go:build ignore

/* SPDX-License-Identifier: (GPL-3.0-only) */

#include "vmlinux.h"
#include "compiler.h"

#include "bpf_tracing.h"
#include "bpf_core_read.h"

#include "bpf_cgroup.h"
#include "bpf_path.h"

enum vfs_mode {
	MODE_READ,
	MODE_WRITE,
	MODE_OPEN,
	MODE_CREATE,
	MODE_MKDIR,
	MODE_UNLINK,
	MODE_RMDIR
};

/* vfs related event key struct */
struct vfs_event_key {
	__u32 cid; /* cgroup ID */
	__u8 mnt[MAX_MOUNT_SIZE]; /* Mount point */
};

/* Any vfs read/write related event */
struct vfs_rw_event {
	__u64 bytes; /* Bytes accumulator */
	__u64 calls; /* Call counter */
	__u64 errors; /* Error counter */
};

/* Any vfs create/open/close/unlink/fsync related event */
struct vfs_inode_event {
	__u64 calls; /* Call counter */
	__u64 errors; /* Error counter */
};

/* 
 * DO NOT USE BPF_F_NO_COMMON_LRU flag while creating maps.
 * This flag ensures that maps are created for each CPU although
 * they can be shared. This means the so-called, active, in-active
 * and free list are kept for each CPU. So, only CPU can evict the
 * entries and this can lead to non LRU behavior. On JZ we noticed
 * that map entries of active jobs are evicted as the processes 
 * have finished on the CPU and this leads to loss of information.
 * 
 * Also using too small max entries can lead to this behavior. 
 * Flag BPF_F_NO_COMMON_LRU is meant to have best performance at
 * the expense of inaccurate LRU behavior. We are more interested
 * in LRU behavior and moreover overhead per bpf call with or
 * without this flag is noticed to be the same. So it is better
 * to omit this flag in prod.
 * 
 * Refs:
 * https://stackoverflow.com/questions/75882443/elements-incorrectly-evicted-from-ebpf-lru-hash-map
 * https://github.com/torvalds/linux/commit/86fe28f7692d96d20232af0fc6d7632d5cc89a01
 * https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/commit/?id=3a08c2fd7634
 * https://docs.ebpf.io/linux/map-type/BPF_MAP_TYPE_LRU_HASH/
 * https://docs.kernel.org/bpf/map_hash.html
*/

/* Map to track vfs_write events */
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_MAP_ENTRIES);
	__type(key, struct vfs_event_key); /* Key is the vfs_event_key struct */
	__type(value, struct vfs_rw_event);
} write_accumulator SEC(".maps");

/* Map to track vfs_read events */
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_MAP_ENTRIES);
	__type(key, struct vfs_event_key); /* Key is the vfs_event_key struct */
	__type(value, struct vfs_rw_event);
} read_accumulator SEC(".maps");

/* Map to track vfs_open events */
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_MAP_ENTRIES);
	__type(key, __u32); /* Key is the vfs_event_key struct */
	__type(value, struct vfs_inode_event);
} open_accumulator SEC(".maps");

/* Map to track vfs_create events */
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_MAP_ENTRIES);
	__type(key, __u32); /* Key is the vfs_event_key struct */
	__type(value, struct vfs_inode_event);
} create_accumulator SEC(".maps");

/* Map to track vfs_unlink events */
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_MAP_ENTRIES);
	__type(key, __u32); /* Key is the vfs_event_key struct */
	__type(value, struct vfs_inode_event);
} unlink_accumulator SEC(".maps");

/**
 * get_mnt_path returns the mount path of the current file.
 * @key: target key
 * @file: file struct
 *
 * Returns size of the mount path.
 */
FUNC_INLINE __u32 get_mnt_path(struct vfs_event_key *key, struct file *file)
{
	int flags = 0, size;
	char *buffer;

	buffer = mnt_path_local(file, &size, &flags);
	if (!buffer)
		return 0;

	asm volatile("%[size] &= 0xff;\n"
		     : [size] "+r"(size));

	bpf_probe_read(key->mnt, sizeof(key->mnt), buffer);

	return (__u32)size;
}

/**
 * is_substring checks if string is part of string.
 * @test_string: string to test
 * @string: string to test against
 *
 * Returns 1 if substring or 0.
 */
FUNC_INLINE int is_substring(const char *test_string, unsigned char string[MAX_MOUNT_SIZE], int size)
{
	unsigned char c1, c2;

#pragma unroll
	for (int i = 0; i < size; ++i) {
		c1 = *test_string++;
		c2 = *string++;
		if (!c1)
			break;
		if (c1 != c2)
			return 0;
	}

	return 1;
}

/**
 * ignore_mnt checks if mount path is in list of ignored mount paths.
 * @mnt: target mount path
 *
 * Returns 1 if mount path to ignore or 0.
 */
FUNC_INLINE int ignore_mnt(unsigned char mnt[MAX_MOUNT_SIZE])
{
	// Ignore /dev, /sys, /proc mounts
	char dev_mnt[] = "/dev";
	char sys_mnt[] = "/sys";
	char proc_mnt[] = "/proc";

	if (is_substring(dev_mnt, mnt, sizeof(dev_mnt)))
		return 1;
	if (is_substring(sys_mnt, mnt, sizeof(dev_mnt)))
		return 1;
	if (is_substring(proc_mnt, mnt, sizeof(dev_mnt)))
		return 1;

	return 0;
}

/**
 * handle_rw_event updates the maps with event by incrementing calls counter
 * and bytes accumulator to the existing event
 * @key: target key
 * @ret: return value of kernel function
 * @type: type of event MODE_READ, MODE_WRITE, etc
 * 
 * The times indicated in the comments are deduced from the tests on a kernel
 * version 5.10 running on 12 core machine (taurus on Grid5000).
 *
 * Returns always 0.
 */
FUNC_INLINE __u64 handle_rw_event(struct file *file, __s64 ret, int type)
{
	// Important to initialise the struct with some values else verifier will complain
	struct vfs_event_key key = { 0 };

	// Get current cgroup ID. Works for both v1 and v2
	// This takes around ~250 ns
	key.cid = (__u32)ceems_get_current_cgroup_id();

	// If cgroup id is 1, it means it is root cgroup and we are not really interested
	// in it and so return
	// Similarly if cgroup id is 0, it means we failed to get cgroup ID
	if (key.cid == 0 || key.cid == 1)
		return 0;

	// Get mount path of the file
	// This takes around ~280 ns
	get_mnt_path(&key, file);
	if (!key.mnt[0])
		return 0;

	// Ignore if found mount is in list of ignored mounts like /sys, /proc, /dev
	//
	// The advantage of ignoring these mounts is that we do not need to populate
	// bpf maps with these stats. This will ensure that we get a closer "LRU"
	// behavior on hash maps.
	//
	// Although in-theory this should avoid updating bpf maps when path is in
	// ignored mounts, on IO intensive processes, we will not see a "big"
	// difference in total execution times as "real" IO calls >>> IO calls to
	// ignored mounts.
	//
	// This takes around ~ 20 ns
	if (ignore_mnt(key.mnt))
		return 0;

	// Creating and/or updating maps takes around ~280 ns
	struct vfs_rw_event *event;

	// Fetch event from correct map
	switch (type) {
	case MODE_WRITE:
		event = bpf_map_lookup_elem(&write_accumulator, &key);
		break;
	case MODE_READ:
		event = bpf_map_lookup_elem(&read_accumulator, &key);
		break;
	default:
		return 0;
	}

	if (!event) {
		// New event with increment call counter
		struct vfs_rw_event new_event = { .bytes = 0, .calls = 1, .errors = 0 };

		// In case of error increment errors else increment bytes
		if (ret < 0) {
			new_event.bytes = 0;
			new_event.errors = 1;
		} else {
			new_event.bytes = (__u64)ret;
			new_event.errors = 0;
		}

		// Update map with new key and event
		switch (type) {
		case MODE_WRITE:
			bpf_map_update_elem(&write_accumulator, &key, &new_event, BPF_NOEXIST);
			break;
		case MODE_READ:
			bpf_map_update_elem(&read_accumulator, &key, &new_event, BPF_NOEXIST);
			break;
		default:
			return 0;
		}

		return 0;
	}

	// Always increment calls
	__sync_fetch_and_add(&event->calls, 1);

	// In case of error increment errors else increment bytes
	if (ret < 0) {
		__sync_fetch_and_add(&event->errors, 1);
	} else {
		__sync_fetch_and_add(&event->bytes, (__u64)ret);
	}

	return 0;
}

/**
 * handle_inode_event updates the maps with event by incrementing calls
 * and errors counters to the existing event
 * @ret: return value of kernel function
 * @type: type of event MODE_OPEN, MODE_CREATE, etc
 *
 * Returns always 0.
 */
FUNC_INLINE __u64 handle_inode_event(__s64 ret, int type)
{
	// Get cgroup ID
	__u32 key = (__u32)ceems_get_current_cgroup_id();

	// If cgroup id is 1, it means it is root cgroup and we are not really interested
	// in it and so return
	// Similarly if cgroup id is 0, it means we failed to get cgroup ID
	if (key == 0 || key == 1)
		return 0;

	struct vfs_inode_event *event;

	// Fetch event from correct map
	switch (type) {
	case MODE_OPEN:
		event = bpf_map_lookup_elem(&open_accumulator, &key);
		break;
	case MODE_CREATE:
		event = bpf_map_lookup_elem(&create_accumulator, &key);
		break;
	case MODE_MKDIR:
		event = bpf_map_lookup_elem(&create_accumulator, &key);
		break;
	case MODE_RMDIR:
		event = bpf_map_lookup_elem(&unlink_accumulator, &key);
		break;
	case MODE_UNLINK:
		event = bpf_map_lookup_elem(&unlink_accumulator, &key);
		break;
	default:
		return 0;
	}

	if (!event) {
		// New event with increment call counter
		struct vfs_inode_event new_event = { .calls = 1, .errors = 0 };

		// In case of error increment errors else increment bytes
		if (ret) {
			new_event.errors = 1;
		}

		// Update map with new key and event
		switch (type) {
		case MODE_OPEN:
			bpf_map_update_elem(&open_accumulator, &key, &new_event, BPF_NOEXIST);
			break;
		case MODE_CREATE:
			bpf_map_update_elem(&create_accumulator, &key, &new_event, BPF_NOEXIST);
			break;
		case MODE_MKDIR:
			bpf_map_update_elem(&create_accumulator, &key, &new_event, BPF_NOEXIST);
			break;
		case MODE_RMDIR:
			bpf_map_update_elem(&unlink_accumulator, &key, &new_event, BPF_NOEXIST);
			break;
		case MODE_UNLINK:
			bpf_map_update_elem(&unlink_accumulator, &key, &new_event, BPF_NOEXIST);
			break;
		default:
			return 0;
		}

		return 0;
	}

	// Always increment calls
	__sync_fetch_and_add(&event->calls, 1);

	// In case of error increment errors else increment bytes
	if (ret) {
		__sync_fetch_and_add(&event->errors, 1);
	}

	return 0;
}
