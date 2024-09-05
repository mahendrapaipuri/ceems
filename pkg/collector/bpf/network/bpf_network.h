//go:build ignore

/* SPDX-License-Identifier: (GPL-3.0-only) */

#include "vmlinux.h"
#include "compiler.h"
#include "net_shared.h"

#include "bpf_tracing.h"
#include "bpf_core_read.h"

#include "bpf_cgroup.h"

enum net_mode {
    MODE_INGRESS,
    MODE_EGRESS
};

/* network related event key struct */
struct net_event_key {
	__u32 cid; /* cgroup ID */
	__u8  dev[16]; /* Device name */
};

/* Any network IPv4/IPv6 related event */
struct net_event {
	__u64 packets; /* Packets counter */
	__u64 bytes; /* Bytes counter */
};

/* Map to track ingress events */
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_MAP_ENTRIES);
	__type(key, struct net_event_key); /* Key is the vfs_event_key struct */
	__type(value, struct net_event);
} ingress_accumulator SEC(".maps");

/* Map to track ingress events */
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_MAP_ENTRIES);
	__type(key, struct net_event_key); /* Key is the net_event_key struct */
	__type(value, struct net_event);
} egress_accumulator SEC(".maps");

/**
 * handle_skb_event updates the maps with event by incrementing packets
 * and bytes counters to the existing event
 * @skb: target skb
 *
 * Returns always 0.
 */
FUNC_INLINE __u64 handle_skb_event(struct sk_buff *skb, int type)
{
	__u32 len;
	struct net_device *dev;

	struct net_event_key key = {0};

	// Get cgroup ID
	key.cid = (__u32) ceems_get_current_cgroup_id();

	// If cgroup id is 1, it means it is root cgroup and we are not really interested
	// in it and so return
	// Similarly if cgroup id is 0, it means we failed to get cgroup ID
	if (key.cid == 0 || key.cid == 1)
		return TC_ACT_OK;

	// Read packet bytes and device name
	bpf_probe_read_kernel(&len, sizeof(len), _(&skb->len));
	bpf_probe_read_kernel(&dev, sizeof(dev), _(&skb->dev));
	bpf_probe_read_kernel_str(&key.dev, sizeof(key.dev), _(&dev->name));

	struct net_event *event;

	// Fetch event from correct map
	switch (type) {
	case MODE_INGRESS:
		event = bpf_map_lookup_elem(&ingress_accumulator, &key);
		break;
	case MODE_EGRESS:
		event = bpf_map_lookup_elem(&egress_accumulator, &key);
		break;
	default:
		return TC_ACT_OK;
	}

	// Get packet size
	__u64 bytes = (__u64) bpf_ntohs(len);

	if (!event) {
		// New event with increment call counter
		struct net_event new_event = { .packets = 1, .bytes = bytes };
        
        // Update map with new key and event
		switch (type) {
		case MODE_INGRESS:
			bpf_map_update_elem(&ingress_accumulator, &key, &new_event, BPF_NOEXIST);
			break;
		case MODE_EGRESS:
			bpf_map_update_elem(&egress_accumulator, &key, &new_event, BPF_NOEXIST);
			break;
		default:
			return TC_ACT_OK;
		}

        return TC_ACT_OK;
    }

    // Always increment calls
	__sync_fetch_and_add(&event->packets, 1);
	__sync_fetch_and_add(&event->bytes, bytes);

    // Let the packet pass
    return TC_ACT_OK;
}
