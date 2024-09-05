//go:build ignore

/* SPDX-License-Identifier: (GPL-3.0-only) */

#include "bpf_network.h"

char __license[] SEC("license") = "GPL";

/**
 * Network related programs.
 * 
 * These are internal kernel functions that we are tracing and the
 * names can be architecture dependent. So we need to check
 * /proc/kallsmys at runtime and check correct function name and
 * hook the program.
 * 
 * If we use fentry (which should be theoritically more performant)
 * we will have to set correct function name at runtime which is 
 * complicated to achieve. So, we use kprobes for these events.
 * 
 * Inital benchmarks showed that fentry/fexit is 100-150 ns faster
 * than kprobes.
 * 
 * However, cilium ebpf refused to load program __netif_receive_skb_core
 * on newer kernels as there is wrapper exported function 
 * netif_receive_skb_core in kernels 6.x.
 * In older kernels there is a bug that is preventing the tracing 
 * functions to access skb pointer.
 * 
 * To avoid more complications, we use ONLY kprobes which should work
 * in all cases.
 * 
 * NOTE that we still need to find architectural specific names 
 * before loading the program by lookin at /proc/kallsyms
*/

SEC("kprobe/__netif_receive_skb_core")
__u64 kprobe___netif_receive_skb_core(struct pt_regs *ctx) 
{
	struct sk_buff *skb;
	bpf_probe_read_kernel(&skb, sizeof(skb), (void *) _(PT_REGS_PARM1(ctx)));

	return handle_skb_event(skb, MODE_INGRESS);
}

SEC("kprobe/__dev_queue_xmit")
__u64 kprobe___dev_queue_xmit(struct pt_regs *ctx) 
{
	struct sk_buff *skb = (struct sk_buff *) PT_REGS_PARM1(ctx);

	return handle_skb_event(skb, MODE_EGRESS);
}
