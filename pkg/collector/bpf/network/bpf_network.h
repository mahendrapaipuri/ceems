//go:build ignore

/* SPDX-License-Identifier: (GPL-3.0-only) */

#include "vmlinux.h"
#include "compiler.h"
#include "net_shared.h"

#include "bpf_tracing.h"
#include "bpf_core_read.h"

#include "bpf_cgroup.h"
#include "bpf_sock.h"

enum net_mode {
	MODE_INGRESS,
	MODE_EGRESS
};

/* 
 * DO NOT USE BPF_F_NO_COMMON_LRU flag while creating maps.
 * See vfs/bpf_vfs.h file for explanations.
*/

/* network related event struct */
struct net_event {
	__u32 cid; /* cgroup ID */
	__u16 proto; /* TCP/UDP */
	__u16 fam; /* sk family AF_INET/AF_INET6 */
};

/* Any network IPv4/IPv6 related stats */
struct net_stats {
	__u64 packets; /* Packets counter */
	__u64 bytes; /* Bytes counter */
};

/* Map to track ingress events */
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_MAP_ENTRIES);
	__type(key, struct net_event); /* Key is the net_event struct */
	__type(value, struct net_stats);
} ingress_accumulator SEC(".maps");

/* Map to track ingress events */
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_MAP_ENTRIES);
	__type(key, struct net_event); /* Key is the net_event struct */
	__type(value, struct net_stats);
} egress_accumulator SEC(".maps");

/* Map to track retransmission events */
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_MAP_ENTRIES);
	__type(key, struct net_event); /* Key is the net_event struct */
	__type(value, struct net_stats);
} retrans_accumulator SEC(".maps");

/**
 * handle_ingress_event updates the maps with ingress event
 * @key: target key to `ingress_accumulator`
 * @stats: `conn_stats` struct to update map
 *
 * Returns always 0.
 */
FUNC_INLINE __u64 handle_ingress_event(struct net_event *key, struct conn_stats *stats)
{
	// Get current ingress stats
	struct net_stats *ingress_stats = bpf_map_lookup_elem(&ingress_accumulator, key);
	if (!ingress_stats) {
		// New event
		struct net_stats new_stats = { .packets = stats->packets_in, .bytes = stats->bytes_received };

		// Update map with new key and event
		bpf_map_update_elem(&ingress_accumulator, key, &new_stats, BPF_NOEXIST);

		return 0;
	}

	// Update map with new stats only when the packets are non zero
	if (stats->packets_in > 0) {
		__sync_fetch_and_add(&ingress_stats->bytes, stats->bytes_received);
		__sync_fetch_and_add(&ingress_stats->packets, stats->packets_in);
	}

	return 0;
}

/**
 * handle_egress_event updates the maps with egress event
 * @key: target key to `retrans_accumulator`
 * @stats: `conn_stats` struct to update map
 *
 * Returns always 0.
 */
FUNC_INLINE __u64 handle_egress_event(struct net_event *key, struct conn_stats *stats)
{
	// Get current egress stats
	struct net_stats *egress_stats = bpf_map_lookup_elem(&egress_accumulator, key);
	if (!egress_stats) {
		// New event with increment call counter
		struct net_stats new_stats = { .packets = stats->packets_out, .bytes = stats->bytes_sent };

		// Update map with new key and event
		bpf_map_update_elem(&egress_accumulator, key, &new_stats, BPF_NOEXIST);

		return 0;
	}

	// Update map with new stats only when the packets are non zero
	if (stats->packets_out > 0) {
		__sync_fetch_and_add(&egress_stats->bytes, stats->bytes_sent);
		__sync_fetch_and_add(&egress_stats->packets, stats->packets_out);
	}

	return 0;
}

/**
 * handle_retrans_event updates the maps with retransmission event
 * @key: target key to `retrans_accumulator`
 * @stats: `conn_stats` struct to update map
 *
 * Returns always 0.
 */
FUNC_INLINE __u64 handle_retrans_event(struct net_event *key, struct conn_stats *stats)
{
	// Get current retrans stats
	struct net_stats *retrans_stats = bpf_map_lookup_elem(&retrans_accumulator, key);
	if (!retrans_stats) {
		// New event with increment call counter
		struct net_stats new_stats = { .packets = stats->total_retrans, .bytes = stats->bytes_retrans };

		// Update map with new key and event
		bpf_map_update_elem(&retrans_accumulator, key, &new_stats, BPF_NOEXIST);

		return 0;
	}

	// Update map with new stats only when the packets are non zero
	if (stats->total_retrans > 0) {
		__sync_fetch_and_add(&retrans_stats->bytes, stats->bytes_retrans);
		__sync_fetch_and_add(&retrans_stats->packets, stats->total_retrans);
	}

	return 0;
}

/**
 * handle_tcp_event updates the maps based on TCP socket events
 * @skp: target `sock` struct
 *
 * Returns always 0.
 */
FUNC_INLINE __u64 handle_tcp_event(struct sock *skp)
{
	struct net_event key = { 0 };

	// Ignore if cgroup ID caanot be found
	key.cid = (__u32)ceems_get_current_cgroup_id();
	if (key.cid == 0)
		return 0;

	/**
	 * We can directly access kernel memory without helpers for fentry/fexit bpf progs
	 * Ref: https://nakryiko.com/posts/bpf-core-reference-guide/#bpf-core-read-1
	 * 
	 * However, we still need to use kprobe/kretprobe for archs other than x86 and
	 * thus we always access memory using helpers to be able to use the helper functions
	 * for all types of probes.
	 */
	key.fam = _sk_family(skp);
	key.proto = (__u16)IPPROTO_TCP;

	// If conn_stats is null, return. There is nothing to update
	struct conn_stats stats = { 0 };
	if (read_conn_stats(&stats, skp)) {
		return 0;
	}

	// Handle ingress, egress and retrans events
	handle_ingress_event(&key, &stats);
	handle_egress_event(&key, &stats);
	handle_retrans_event(&key, &stats);

	return 0;
}

/**
 * handle_udp_event updates the maps based on UDP socket events
 * @ret: return value of kernel function. Either size or error
 *
 * Returns always 0.
 */
FUNC_INLINE __u64 handle_udp_event(int ret, int family, int type)
{
	// Negative return value means failed event
	if (ret <= 0)
		return 0;

	__u64 bytes = (__u64)ret;
	struct net_event key = { 0 };

	// Ignore if cgroup ID caanot be found
	key.cid = (__u32)ceems_get_current_cgroup_id();
	if (key.cid == 0)
		return 0;

	key.fam = (__u16)family;
	key.proto = (__u16)IPPROTO_UDP;

	// Fetch event from correct map
	struct net_stats *stats;
	switch (type) {
	case MODE_INGRESS:
		stats = bpf_map_lookup_elem(&ingress_accumulator, &key);
		break;
	case MODE_EGRESS:
		stats = bpf_map_lookup_elem(&egress_accumulator, &key);
		break;
	default:
		return 0;
	}

	if (!stats) {
		// New event with increment call counter
		struct net_stats new_stats = { .packets = 1, .bytes = bytes };

		// Update map with new key and event
		switch (type) {
		case MODE_INGRESS:
			bpf_map_update_elem(&ingress_accumulator, &key, &new_stats, BPF_NOEXIST);
			break;
		case MODE_EGRESS:
			bpf_map_update_elem(&egress_accumulator, &key, &new_stats, BPF_NOEXIST);
			break;
		default:
			return 0;
		}

		return 0;
	}

	// Increment packets and bytes
	__sync_fetch_and_add(&stats->packets, 1);
	__sync_fetch_and_add(&stats->bytes, bytes);

	return 0;
}
