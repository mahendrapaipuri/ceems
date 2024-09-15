//go:build ignore

/* SPDX-License-Identifier: (GPL-3.0-only) */

#include "bpf_network.h"

char __license[] SEC("license") = "GPL";

/**
 * Network related programs.
 * 
 * Currently we are monitoring TCP and UDP traffics for both
 * IPv4 and IPv6.
 * 
 * The change in function signatures for certain functions are
 * taken into account. Similarly, `tcp_sendpage` and `udp_sendpage`
 * have been removed in kernel 6.5 and it has been taken into
 * account as well. The functions we are using to trace here are
 * exported functions and their names should not change with
 * any kernel related optimizations.
 * 
 * For x86_64, fentry/fexit are used which give max performance and
 * for the rest of architectures, kprobes/kretprobes are used.
 * 
 * Inital benchmarks tcp_sendmsg probe takes 1200ns/call whereas
 * tcp_recvmsg probe takes 6000ns/call. More benchmarks to do to
 * measure overhead on other probes. These tests are made using
 * bpftool by setting `sysctl -w kernel.bpf_stats_enabled=1`.
 * Note that there is a overhead of around 20-30ns due to 
 * instrumentation.
 * 
*/

#if defined(__TARGET_ARCH_x86)

SEC("fexit/tcp_sendmsg")
__u64 BPF_PROG(fexit_tcp_sendmsg, struct sock *sk, struct msghdr *msg, size_t size, int ret)
{
	return handle_tcp_event(sk);
}

SEC("fexit/udp_sendmsg")
__u64 BPF_PROG(fexit_udp_sendmsg, struct sock *sk, struct msghdr *msg, size_t size, int ret)
{
	return handle_udp_event(ret, AF_INET, MODE_EGRESS);
}

SEC("fexit/udpv6_sendmsg")
__u64 BPF_PROG(fexit_udpv6_sendmsg, struct sock *sk, struct msghdr *msg, size_t size, int ret)
{
	return handle_udp_event(ret, AF_INET6, MODE_EGRESS);
}

#if defined(__KERNEL_PRE_v64)

SEC("fexit/tcp_sendpage")
__u64 BPF_PROG(fexit_tcp_sendpage, struct sock *sk, struct page *page, int offset, size_t size, int flags, int ret)
{
	return handle_tcp_event(sk);
}

SEC("fexit/udp_sendpage")
__u64 BPF_PROG(fexit_udp_sendpage, struct sock *sk, struct page *page, int offset, size_t size, int flags, int ret)
{
	return handle_udp_event(ret, AF_INET, MODE_EGRESS);
}

#endif

#if defined(__KERNEL_PRE_v519)

SEC("fexit/tcp_recvmsg")
__u64 BPF_PROG(fexit_tcp_recvmsg, struct sock *sk, struct msghdr *msg, size_t size, int noblock, int flags, int *addr_len, int ret)
{
	return handle_tcp_event(sk);
}

SEC("fexit/udp_recvmsg")
__u64 BPF_PROG(fexit_udp_recvmsg, struct sock *sk, struct msghdr *msg, size_t size, int noblock, int flags, int *addr_len, int ret)
{
	return handle_udp_event(ret, AF_INET, MODE_INGRESS);
}

SEC("fexit/udpv6_recvmsg")
__u64 BPF_PROG(fexit_udpv6_recvmsg, struct sock *sk, struct msghdr *msg, size_t size, int noblock, int flags, int *addr_len, int ret)
{
	return handle_udp_event(ret, AF_INET6, MODE_INGRESS);
}

#else

SEC("fexit/tcp_recvmsg")
__u64 BPF_PROG(fexit_tcp_recvmsg, struct sock *sk, struct msghdr *msg, size_t size, int flags, int *addr_len, int ret)
{
	return handle_tcp_event(sk);
}

SEC("fexit/udp_recvmsg")
__u64 BPF_PROG(fexit_udp_recvmsg, struct sock *sk, struct msghdr *msg, size_t size, int flags, int *addr_len, int ret)
{
	return handle_udp_event(ret, AF_INET, MODE_INGRESS);
}

SEC("fexit/udpv6_recvmsg")
__u64 BPF_PROG(fexit_udpv6_recvmsg, struct sock *sk, struct msghdr *msg, size_t size, int flags, int *addr_len, int ret)
{
	return handle_udp_event(ret, AF_INET6, MODE_INGRESS);
}

#endif

#else

SEC("kprobe/tcp_sendmsg")
__u64 kprobe_tcp_sendmsg(struct pt_regs *ctx)
{
	struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);

	return handle_tcp_event(sk);
}

SEC("kretprobe/udp_sendmsg")
__u64 kretprobe_udp_sendmsg(struct pt_regs *ctx)
{
	int ret = (int)PT_REGS_RC(ctx);

	return handle_udp_event(ret, AF_INET, MODE_EGRESS);
}

SEC("kretprobe/udpv6_sendmsg")
__u64 kretprobe_udpv6_sendmsg(struct pt_regs *ctx)
{
	int ret = (int)PT_REGS_RC(ctx);

	return handle_udp_event(ret, AF_INET6, MODE_EGRESS);
}

#if defined(__KERNEL_PRE_v64)

SEC("kprobe/tcp_sendpage")
__u64 kprobe_tcp_sendpage(struct pt_regs *ctx)
{
	struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);

	return handle_tcp_event(sk);
}

SEC("kretprobe/udp_sendpage")
__u64 kretprobe_udp_sendpage(struct pt_regs *ctx)
{
	int ret = (int)PT_REGS_RC(ctx);

	return handle_udp_event(ret, AF_INET, MODE_EGRESS);
}

#endif

#if defined(__KERNEL_PRE_v519)

SEC("kprobe/tcp_recvmsg")
__u64 kprobe_tcp_recvmsg(struct pt_regs *ctx)
{
	struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);

	return handle_tcp_event(sk);
}

SEC("kretprobe/udp_recvmsg")
__u64 kretprobe_udp_recvmsg(struct pt_regs *ctx)
{
	int ret = (int)PT_REGS_RC(ctx);

	return handle_udp_event(ret, AF_INET, MODE_INGRESS);
}

SEC("kretprobe/udpv6_recvmsg")
__u64 kretprobe_udpv6_recvmsg(struct pt_regs *ctx)
{
	int ret = (int)PT_REGS_RC(ctx);

	return handle_udp_event(ret, AF_INET6, MODE_INGRESS);
}

#else

SEC("kprobe/tcp_recvmsg")
__u64 kprobe_tcp_recvmsg(struct pt_regs *ctx)
{
	struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);

	return handle_tcp_event(sk);
}

SEC("kretprobe/udp_recvmsg")
__u64 kretprobe_udp_recvmsg(struct pt_regs *ctx)
{
	int ret = (int)PT_REGS_RC(ctx);

	return handle_udp_event(ret, AF_INET, MODE_INGRESS);
}

SEC("kretprobe/udpv6_recvmsg")
__u64 kretprobe_udpv6_recvmsg(struct pt_regs *ctx)
{
	int ret = (int)PT_REGS_RC(ctx);

	return handle_udp_event(ret, AF_INET6, MODE_INGRESS);
}

#endif

#endif
