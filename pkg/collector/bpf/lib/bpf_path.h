/* SPDX-License-Identifier: (GPL-3.0-only) */
// Nicked a lot of utility functions from https://github.com/cilium/tetragon project

#ifndef _BPF_VFS_EVENT__
#define _BPF_VFS_EVENT__

/* __d_path_local flags */
// #define UNRESOLVED_MOUNT_POINTS	   0x01 // (deprecated)
// this error is returned by __d_path_local in the following cases:
// - the path walk did not conclude (too many dentry)
// - the path was too long to fit in the buffer
#define UNRESOLVED_PATH_COMPONENTS 0x02

#define PROBE_MNT_ITERATIONS 8
#define ENAMETOOLONG 36 /* File name too long */
#define MAX_BUF_LEN 4096

/* buffer in the heap */
struct buffer_heap_map_value {
	unsigned char buf[MAX_BUF_LEN + 256];
};

/* per CPU buffer map for storing resolved mount path */
struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(max_entries, 1);
	__type(key, int);
	__type(value, struct buffer_heap_map_value);
} buffer_heap_map SEC(".maps");

/* struct to keep mount path data */
struct mnt_path_data {
	char *bf;
	struct mount *mnt;
	struct dentry *prev_de;
	char *bptr;
	int blen;
	bool resolved;
};

#define offsetof_btf(s, memb) ((size_t)((char *)_(&((s *)0)->memb) - (char *)0))

#define container_of_btf(ptr, type, member)                      \
	({                                                       \
		void *__mptr = (void *)(ptr);                    \
		((type *)(__mptr - offsetof_btf(type, member))); \
	})

/**
 * real_mount() returns real mount path of the current vfsmount.
 * @mnt: pointer to vfsmount of the path
 *
 * Returns pointer to mnt of real mount path.
 */
FUNC_INLINE struct mount* real_mount(struct vfsmount *mnt)
{
	return container_of_btf(mnt, struct mount, mnt);
}

/**
 * IS_ROOT() returns true if the current path reached root level.
 * @dentry: pointer to dentry of the path
 *
 * Returns true if current dentry is the root level.
 */
FUNC_INLINE bool IS_ROOT(struct dentry *dentry)
{
	struct dentry *d_parent;

	bpf_probe_read(&d_parent, sizeof(d_parent), _(&dentry->d_parent));

	return (dentry == d_parent);
}

/**
 * prepend_name() prepends the mount path name for current path.
 * @buf: buffer where name will be prepended
 * @bufptr: pointer to the buf
 * @buflen: current buffer length
 * @name: name to be prepended to the buffer
 * @namelen: length of name
 *
 * Returns 0 on update and -ENAMETOOLONG on errors.
 */
FUNC_INLINE int prepend_name(char *buf, char **bufptr, int *buflen, const unsigned char *name, int namelen)
{
	// contains 1 if the buffer is large enough to contain the whole name and a slash prefix
	bool write_slash = 1;

	// Ensure namelen will not overflow. To make verifier happy
	if (namelen < 0 || namelen > 256)
		return -ENAMETOOLONG;

	s64 buffer_offset = (s64)(*bufptr) - (s64)buf;

	// Change name and namelen to fit in the buffer.
	// We prefer to store the part of it that fits rather than discard it.
	if (namelen >= *buflen) {
		name += namelen - *buflen;
		namelen = *buflen;
		write_slash = 0;
	}

	*buflen -= (namelen + write_slash);

	if (namelen + write_slash > buffer_offset)
		return -ENAMETOOLONG;

	buffer_offset -= (namelen + write_slash);

	// This will never happen. buffer_offset is the diff of the initial buffer pointer
	// with the current buffer pointer. This will be at max 4096 bytes (similar to the initial
	// size).
	// Needed to bound that for bpf_probe_read call.
	if (buffer_offset < 0 || buffer_offset >= MAX_BUF_LEN)
		return -ENAMETOOLONG;

	if (write_slash)
		buf[buffer_offset] = '/';

	// This ensures that namelen is < 256, which is aligned with kernel's max dentry name length
	// that is 255 (https://elixir.bootlin.com/linux/v5.10/source/include/uapi/linux/limits.h#L12).
	// Needed to bound that for probe_read call.
	asm volatile("%[namelen] &= 0xff;\n"
		     : [namelen] "+r"(namelen));
	bpf_probe_read(buf + buffer_offset + write_slash, namelen * sizeof(const unsigned char), name);

	*bufptr = buf + buffer_offset;

	return write_slash ? 0 : -ENAMETOOLONG;
}

/**
 * mnt_path_read() updates path buffer with current mount path.
 * @data: pointer to mount path data for current recursion level
 *
 * Returns 0 on update and 1 on successful path resolution and any errors.
 */
FUNC_INLINE long mnt_path_read(struct mnt_path_data *data)
{
	struct dentry *curr_de;
	struct dentry *prev_de = data->prev_de;
	struct mount *mnt = data->mnt;
	struct mount *mnt_parent;
	const unsigned char *name;
	int len;
	int error;

	bpf_probe_read(&curr_de, sizeof(curr_de), _(&mnt->mnt_mountpoint));
	
	/* Global root? */
	if (curr_de == prev_de || IS_ROOT(curr_de)) {

		// resolved all path components successfully
		data->resolved = true;

		return 1;
	}
	bpf_probe_read(&name, sizeof(name), _(&curr_de->d_name.name));
	bpf_probe_read(&len, sizeof(len), _(&curr_de->d_name.len));
	bpf_probe_read(&mnt_parent, sizeof(mnt_parent), _(&mnt->mnt_parent));

	error = prepend_name(data->bf, &data->bptr, &data->blen, name, len);
	// This will happen where the dentry name does not fit in the buffer.
	// We will stop the loop with resolved == false and later we will
	// set the proper value in error before function return.
	if (error)
		return 1;

	data->prev_de = curr_de;
	data->mnt = mnt_parent;

	return 0;
}

/**
 * Convience wrapper for mnt_path_read() to be used in BPF loop helper
 */
#if defined(__KERNEL_POST_v62)
static long mnt_path_read_v61(__u32 index, void *data)
{
	return mnt_path_read(data);
}
#endif

/**
 * prepend_mnt_path() returns the mount path of the file.
 * @file: pointer to a file that we want to resolve
 * @buf: buffer where the path will be stored (this should be always the value of 'buffer_heap_map' map)
 * @buflen: available buffer size to store the path (now 256 in all cases, maybe we can increase that further)
 *
 * Returns error code on failures.
 */
FUNC_INLINE int prepend_mnt_path(struct file *file, char *bf, char **buffer, int *buflen)
{
	struct mnt_path_data data = {
		.bf = bf,
		.bptr = *buffer,
		.blen = *buflen,
		.prev_de = NULL,
	};
	int error = 0;
	struct vfsmount *vfsmnt;

	bpf_probe_read(&vfsmnt, sizeof(vfsmnt), _(&file->f_path.mnt));
	data.mnt = real_mount(vfsmnt);

#if defined(__KERNEL_POST_v62)
	bpf_loop(PROBE_MNT_ITERATIONS, mnt_path_read_v61, (void *)&data, 0);
#else
#pragma unroll
	for (int i = 0; i < PROBE_MNT_ITERATIONS; ++i) {
		if (mnt_path_read(&data))
			break;
	}
#endif /* __KERNEL_POST_v62 */

	if (data.bptr == *buffer) {
		*buflen = 0;

		return 0;
	}

	if (!data.resolved)
		error = UNRESOLVED_PATH_COMPONENTS;

	*buffer = data.bptr;
	*buflen = data.blen;
    
	return error;
}

/**
 * __mnt_path_local() returns the mount path of the file.
 * @file: pointer to a file that we want to resolve
 * @buf: buffer where the path will be stored (this should be always the value of 'buffer_heap_map' map)
 * @buflen: available buffer size to store the path (now 256 in all cases, maybe we can increase that further)
 *
 * Input buffer layout:
 * <--        buflen         -->
 * -----------------------------
 * |                           |
 * -----------------------------
 * ^
 * |
 * buf
 *
 *
 * Output variables:
 * - 'buf' is where the path is stored (>= compared to the input argument)
 * - 'buflen' the size of the resolved path (0 < buflen <= 256). Will not be negative. If buflen == 0 nothing is written to the buffer.
 * - 'error' 0 in case of success or UNRESOLVED_PATH_COMPONENTS in the case where the path is larger than the provided buffer.
 *
 * Output buffer layout:
 * <--   buflen  -->
 * -----------------------------
 * |                /etc/passwd|
 * -----------------------------
 *                 ^
 *                 |
 *                buf
 *
 * ps. The size of the path will be (initial value of buflen) - (return value of buflen) if (buflen != 0)
 */
FUNC_INLINE char* __mnt_path_local(struct file *file, char *buf, int *buflen, int *error)
{
	char *res = buf + *buflen;

	*error = prepend_mnt_path(file, buf, &res, buflen);

	return res;
}

/**
 * Entry point for mount path resolution.
 *
 * This function allocates a buffer from 'buffer_heap_map' map and calls
 * __mnt_path_local. After __mnt_path_local returns, it also does the appropriate
 * calculations on the buffer size (check __mnt_path_local comment).
 *
 * Returns the buffer where the path is stored. 'buflen' is the size of the
 * resolved path (0 < buflen <= 256) and will not be negative. If buflen == 0
 * nothing is written to the buffer (still the value to the buffer is valid).
 * 'error' is 0 in case of success or UNRESOLVED_PATH_COMPONENTS in the case
 * where the path is larger than the provided buffer.
 */
FUNC_INLINE char* mnt_path_local(struct file *file, int *buflen, int *error)
{
    int zero = 0;
	char *buffer = 0;

	buffer = bpf_map_lookup_elem(&buffer_heap_map, &zero);
	if (!buffer)
		return 0;

	*buflen = MAX_BUF_LEN;
	buffer = __mnt_path_local(file, buffer, buflen, error);
	if (*buflen > 0)
		*buflen = MAX_BUF_LEN - *buflen;

	return buffer;
}

#endif
