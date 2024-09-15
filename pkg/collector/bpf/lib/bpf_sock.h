//go:build ignore

/* SPDX-License-Identifier: (GPL-3.0-only) */

#define MAX_SOCKET_CONN_ENTRIES 2048

/* conn_event represents a socket connection */
struct conn_event {
	__u64 saddr_h;
	__u64 saddr_l;
	__u64 daddr_h;
	__u64 daddr_l;
	__u16 sport;
	__u16 dport;
};

/* Socket related stats */
struct conn_stats {
	__u64 packets_in; /* Ingress packets counter */
	__u64 packets_out; /* Egress packets counter */
	__u64 bytes_received; /* Ingress bytes */
	__u64 bytes_sent; /* Egress bytes */
	__u64 total_retrans; /* Retransmissions counter */
	__u64 bytes_retrans; /* Retransmissions bytes */
};

/* Map to track socket connections */
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_SOCKET_CONN_ENTRIES);
	__type(key, struct conn_event); /* Key is the conn_event struct */
	__type(value, struct conn_stats);
} socket_accumulator SEC(".maps");

/** 
 * is_ipv4_mapped_ipv6 checks if IPs are IPv4 mapped to IPv6 ::ffff:xxxx:xxxx
 * https://tools.ietf.org/html/rfc4291#section-2.5.5
 * the addresses are stored in network byte order so IPv4 adddress is stored
 * in the most significant 32 bits of part saddr_l and daddr_l.
 * Meanwhile the end of the mask is stored in the least significant 32 bits.
 */
FUNC_INLINE bool is_ipv4_mapped_ipv6(__u64 saddr_h, __u64 saddr_l, __u64 daddr_h, __u64 daddr_l)
{
#if __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
	return ((saddr_h == 0 && ((__u32)saddr_l == 0xFFFF0000)) || (daddr_h == 0 && ((__u32)daddr_l == 0xFFFF0000)));
#elif __BYTE_ORDER__ == __ORDER_BIG_ENDIAN__
	return ((saddr_h == 0 && ((__u32)(saddr_l >> 32) == 0x0000FFFF)) || (daddr_h == 0 && ((__u32)(daddr_l >> 32) == 0x0000FFFF)));
#else
#error "Fix your compiler's __BYTE_ORDER__?!"
#endif
}

/** 
 * tcp_sk casts sock struct into `tcp_sock` struct
 * @sk: sock struct
 * 
 * Returns pointer to `tcp_sock` struct
 */
FUNC_INLINE struct tcp_sock *tcp_sk(const struct sock *sk)
{
	return (struct tcp_sock *)sk;
}

/** 
 * inet_sk casts sock struct into `inet_sock` struct
 * @sk: sock struct
 * 
 * Returns pointer to `inet_sock` struct
 */
FUNC_INLINE struct inet_sock *inet_sk(const struct sock *sk)
{
	return (struct inet_sock *)sk;
}

/** 
 * read_in6_addr reads ipv6 address from `in6` struct
 */
FUNC_INLINE void read_in6_addr(__u64 *addr_h, __u64 *addr_l, const struct in6_addr *in6)
{
	bpf_probe_read_kernel(addr_h, sizeof(addr_h), _(&in6->in6_u.u6_addr32[0]));
	bpf_probe_read_kernel(addr_l, sizeof(addr_l), _(&in6->in6_u.u6_addr32[2]));
}

/** 
 * read_sport reads source port from `sock` struct
 * @sk: `sock` struct
 * 
 * Returns source port in host byte order
 */
FUNC_INLINE __u16 read_sport(struct sock *skp)
{
	// try skc_num, then inet_sport
	__u16 sport;
	bpf_probe_read_kernel(&sport, sizeof(sport), _(&skp->__sk_common.skc_num));
	if (sport == 0) {
		struct inet_sock *inet_sock = inet_sk(skp);
		bpf_probe_read_kernel(&sport, sizeof(sport), _(&inet_sock->inet_sport));
		sport = bpf_ntohs(sport);
	}

	return sport;
}

/** 
 * read_dport reads destination port from `sock` struct
 * @sk: `sock` struct
 * 
 * Returns destination port in host byte order
 */
FUNC_INLINE __u16 read_dport(struct sock *skp)
{
	__u16 dport;
	bpf_probe_read_kernel(&dport, sizeof(dport), _(&skp->__sk_common.skc_dport));
	if (dport == 0) {
		struct inet_sock *inet_sock = inet_sk(skp);
		bpf_probe_read_kernel(&dport, sizeof(dport), _(&inet_sock->sk.__sk_common.skc_dport));
	}

	return bpf_ntohs(dport);
}

/** 
 * read_saddr_v4 reads source ipv4 address from `sock` struct
 * @sk: `sock` struct
 * 
 * Returns source ipv4 address
 */
FUNC_INLINE __u32 read_saddr_v4(struct sock *skp)
{
	__u32 saddr;
	bpf_probe_read_kernel(&saddr, sizeof(saddr), _(&skp->__sk_common.skc_rcv_saddr));
	if (saddr == 0) {
		struct inet_sock *inet_sockp = inet_sk(skp);
		bpf_probe_read_kernel(&saddr, sizeof(saddr), _(&inet_sockp->inet_saddr));
	}

	return saddr;
}

/** 
 * read_daddr_v4 reads destination ipv4 address from `sock` struct
 * @sk: `sock` struct
 * 
 * Returns destination ipv4 address
 */
FUNC_INLINE __u32 read_daddr_v4(struct sock *skp)
{
	__u32 daddr;
	bpf_probe_read_kernel(&daddr, sizeof(daddr), _(&skp->__sk_common.skc_daddr));
	if (daddr == 0) {
		struct inet_sock *inet_sock = inet_sk(skp);
		bpf_probe_read_kernel(&daddr, sizeof(daddr), _(&inet_sock->sk.__sk_common.skc_daddr));
	}

	return daddr;
}

/** 
 * read_saddr_v6 reads source ipv6 address from `sock` struct
 * @sk: `sock` struct
 * 
 * Returns none
 */
FUNC_INLINE void read_saddr_v6(struct sock *skp, __u64 *addr_h, __u64 *addr_l)
{
	struct in6_addr in6;
	bpf_probe_read_kernel(&in6, sizeof(in6), _(&skp->__sk_common.skc_v6_rcv_saddr));
	read_in6_addr(addr_h, addr_l, &in6);
}

/** 
 * read_daddr_v6 reads destination ipv6 address from `sock` struct
 * @sk: `sock` struct
 * 
 * Returns none
 */
FUNC_INLINE void read_daddr_v6(struct sock *skp, __u64 *addr_h, __u64 *addr_l)
{
	struct in6_addr in6;
	bpf_probe_read_kernel(&in6, sizeof(in6), _(&skp->__sk_common.skc_v6_daddr));
	read_in6_addr(addr_h, addr_l, &in6);
}

/** 
 * _sk_family reads socket family from `sock` struct
 * @sk: `sock` struct
 * 
 * Returns socket family
 */
FUNC_INLINE __u16 _sk_family(struct sock *skp)
{
	__u16 family;
	bpf_probe_read_kernel(&family, sizeof(family), _(&skp->__sk_common.skc_family));
	return family;
}

/**
 * read_conn_tuple reads values into a `conn_event` from a `sock` struct. 
 * @t: `conn_event` struct
 * @skp: `sock` struct
 * 
 * Returns 0 success, 1 otherwise.
 */
FUNC_INLINE int read_conn_tuple(struct conn_event *t, struct sock *skp)
{
	int err = 0;

	u16 family = _sk_family(skp);
	// Retrieve addresses
	if (family == AF_INET) {
		if (t->saddr_l == 0) {
			t->saddr_l = read_saddr_v4(skp);
		}
		if (t->daddr_l == 0) {
			t->daddr_l = read_daddr_v4(skp);
		}

		if (t->saddr_l == 0 || t->daddr_l == 0) {
			err = 1;
		}
	} else if (family == AF_INET6) {
		if (!(t->saddr_h || t->saddr_l)) {
			read_saddr_v6(skp, &t->saddr_h, &t->saddr_l);
		}
		if (!(t->daddr_h || t->daddr_l)) {
			read_daddr_v6(skp, &t->daddr_h, &t->daddr_l);
		}

		if (!(t->saddr_h || t->saddr_l)) {
			err = 1;
		}

		if (!(t->daddr_h || t->daddr_l)) {
			err = 1;
		}

		// Check if we can map IPv6 to IPv4
		if (is_ipv4_mapped_ipv6(t->saddr_h, t->saddr_l, t->daddr_h, t->daddr_l)) {
			t->saddr_h = 0;
			t->daddr_h = 0;
			t->saddr_l = (__u32)(t->saddr_l >> 32);
			t->daddr_l = (__u32)(t->daddr_l >> 32);
		}
	} else {
		err = 1;
	}

	// Retrieve ports
	if (t->sport == 0) {
		t->sport = read_sport(skp);
	}
	if (t->dport == 0) {
		t->dport = read_dport(skp);
	}

	if (t->sport == 0 || t->dport == 0) {
		err = 1;
	}

	return err;
}

/**
 * read_conn_stats reads incremental stats into a `conn_stats` for a `sock` struct. 
 * @t: `conn_stats` struct
 * @skp: `sock` struct
 * 
 * Returns 0 success, 1 otherwise.
 */
FUNC_INLINE int read_conn_stats(struct conn_stats *incr_stats, struct sock *skp)
{
	// Read current socket connection
	struct conn_event t = { 0 };
	if (read_conn_tuple(&t, skp))
		return 1;

	// Read socket connection stats
	// IMPORTANT to read them into correct types and then later cast into our custom types
	__u32 packets_in, packets_out, total_retrans;
	__u64 bytes_received, bytes_sent, bytes_retrans;

	// Cast into tcp_sock struct to read packets and bytes
	struct tcp_sock *tcp_skp = tcp_sk(skp);

	// Use helpers to read kernel memory
	bpf_probe_read_kernel(&packets_in, sizeof(packets_in), _(&tcp_skp->segs_in));
	bpf_probe_read_kernel(&packets_out, sizeof(packets_out), _(&tcp_skp->segs_out));
	bpf_probe_read_kernel(&bytes_received, sizeof(bytes_received), _(&tcp_skp->bytes_received));
	bpf_probe_read_kernel(&bytes_sent, sizeof(bytes_sent), _(&tcp_skp->bytes_sent));
	bpf_probe_read_kernel(&total_retrans, sizeof(total_retrans), _(&tcp_skp->total_retrans));
	bpf_probe_read_kernel(&bytes_retrans, sizeof(bytes_retrans), _(&tcp_skp->bytes_retrans));

	struct conn_stats *stats = bpf_map_lookup_elem(&socket_accumulator, &t);
	if (!stats) {
		// Update stats
		incr_stats->packets_in = (__u64)packets_in;
		incr_stats->packets_out = (__u64)packets_out;
		incr_stats->bytes_received = bytes_received;
		incr_stats->bytes_sent = bytes_sent;
		incr_stats->total_retrans = (__u64)total_retrans;
		incr_stats->bytes_retrans = bytes_retrans;

		// Update map
		bpf_map_update_elem(&socket_accumulator, &t, incr_stats, BPF_NOEXIST);

		return 0;
	}

	// Update incr_stats
	incr_stats->packets_in = (__u64)packets_in - stats->packets_in;
	incr_stats->packets_out = (__u64)packets_out - stats->packets_out;
	incr_stats->bytes_received = bytes_received - stats->bytes_received;
	incr_stats->bytes_sent = bytes_sent - stats->bytes_sent;
	incr_stats->total_retrans = (__u64)total_retrans - stats->total_retrans;
	incr_stats->bytes_retrans = bytes_retrans - stats->bytes_retrans;

	// Update map with new counters
	stats->packets_in = (__u64)packets_in;
	stats->packets_out = (__u64)packets_out;
	stats->bytes_received = bytes_received;
	stats->bytes_sent = bytes_sent;
	stats->total_retrans = (__u64)total_retrans;
	stats->bytes_retrans = bytes_retrans;

	return 0;
}
