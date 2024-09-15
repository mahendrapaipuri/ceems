//go:build ignore

/* SPDX-License-Identifier: (GPL-3.0-only) */

#include "bpf_vfs.h"

char __license[] SEC("license") = "GPL";

/**
 * VFS related events.
 * 
 * BPF trampolines are implemented in ARM64 with limited
 * functionality only in kernel version 6.0.
 * 
 * So we use fentry/fexit only for x86 architecture and for
 * the rest we use kprobe/kretprobes.
 */
#if defined(__TARGET_ARCH_x86)

SEC("fexit/vfs_write")
__u64 BPF_PROG(fexit_vfs_write, struct file *file,
	       const char __user *buf, size_t count, loff_t *pos, ssize_t ret)
{
	return handle_rw_event(file, (__s64)ret, MODE_WRITE);
}

SEC("fexit/vfs_read")
__u64 BPF_PROG(fexit_vfs_read, struct file *file,
	       char __user *buf, size_t count, loff_t *pos, ssize_t ret)
{
	return handle_rw_event(file, (__s64)ret, MODE_READ);
}

SEC("fexit/vfs_writev")
__u64 BPF_PROG(fexit_vfs_writev, struct file *file,
	       const char __user *buf, size_t count, loff_t *pos, ssize_t ret)
{
	return handle_rw_event(file, (__s64)ret, MODE_WRITE);
}

SEC("fexit/vfs_readv")
__u64 BPF_PROG(fexit_vfs_readv, struct file *file,
	       char __user *buf, size_t count, loff_t *pos, ssize_t ret)
{
	return handle_rw_event(file, (__s64)ret, MODE_READ);
}

SEC("fexit/vfs_open")
__u64 BPF_PROG(fexit_vfs_open, const struct path *path, struct file *file, int ret)
{
	return handle_inode_event((__s64)ret, MODE_OPEN);
}

/**
 * Functions vfs_create, vfs_open, vfs_rmdir vfs_mkdir and vfs_unlink 
 * have different function signatures depending on kernel version.
 * 
 * The current pre-processors will compile different programs based
 * on the kernel version.
 * 
 * From initial benchmark tests the difference between fexit and kretprobe
 * is around 100-150 ns in the favour of fexit (as expected).
 */
#if defined(__KERNEL_PRE_v511)

SEC("fexit/vfs_create")
__u64 BPF_PROG(fexit_vfs_create, struct inode *dir, struct dentry *dentry, umode_t mode,
	       bool want_excl, int ret)
{
	return handle_inode_event((__s64)ret, MODE_CREATE);
}

SEC("fexit/vfs_mkdir")
__u64 BPF_PROG(fexit_vfs_mkdir, struct inode *dir, struct dentry *dentry, umode_t mode,
	       int ret)
{
	return handle_inode_event((__s64)ret, MODE_MKDIR);
}

SEC("fexit/vfs_unlink")
__u64 BPF_PROG(fexit_vfs_unlink, struct inode *dir, struct dentry *dentry,
	       struct inode **pdir, int ret)
{
	return handle_inode_event((__s64)ret, MODE_UNLINK);
}

SEC("fexit/vfs_rmdir")
__u64 BPF_PROG(fexit_vfs_rmdir, struct inode *dir, struct dentry *dentry, int ret)
{
	return handle_inode_event((__s64)ret, MODE_RMDIR);
}

#elif defined(__KERNEL_POST_v512_PRE_v62)

SEC("fexit/vfs_create")
__u64 BPF_PROG(fexit_vfs_create, struct user_namespace *mnt_userns, struct inode *dir,
	       struct dentry *dentry, umode_t mode, bool want_excl, int ret)
{
	return handle_inode_event((__s64)ret, MODE_CREATE);
}

SEC("fexit/vfs_mkdir")
__u64 BPF_PROG(fexit_vfs_mkdir, struct user_namespace *mnt_userns, struct inode *dir,
	       struct dentry *dentry, umode_t mode, int ret)
{
	return handle_inode_event((__s64)ret, MODE_MKDIR);
}

SEC("fexit/vfs_unlink")
__u64 BPF_PROG(fexit_vfs_unlink, struct user_namespace *mnt_userns, struct inode *dir,
	       struct dentry *dentry, struct inode **pdir, int ret)
{
	return handle_inode_event((__s64)ret, MODE_UNLINK);
}

SEC("fexit/vfs_rmdir")
__u64 BPF_PROG(fexit_vfs_rmdir, struct user_namespace *mnt_userns, struct inode *dir,
	       struct dentry *dentry, int ret)
{
	return handle_inode_event((__s64)ret, MODE_RMDIR);
}

#else

SEC("fexit/vfs_create")
__u64 BPF_PROG(fexit_vfs_create, struct mnt_idmap *idmap, struct inode *dir,
	       struct dentry *dentry, umode_t mode, bool want_excl, int ret)
{
	return handle_inode_event((__s64)ret, MODE_CREATE);
}

SEC("fexit/vfs_mkdir")
__u64 BPF_PROG(fexit_vfs_mkdir, struct mnt_idmap *idmap, struct inode *dir,
	       struct dentry *dentry, umode_t mode, int ret)
{
	return handle_inode_event((__s64)ret, MODE_MKDIR);
}

SEC("fexit/vfs_unlink")
__u64 BPF_PROG(fexit_vfs_unlink, struct mnt_idmap *idmap, struct inode *dir,
	       struct dentry *dentry, struct inode **pdir, int ret)
{
	return handle_inode_event((__s64)ret, MODE_UNLINK);
}

SEC("fexit/vfs_rmdir")
__u64 BPF_PROG(fexit_vfs_rmdir, struct mnt_idmap *idmap, struct inode *dir,
	       struct dentry *dentry, int ret)
{
	return handle_inode_event((__s64)ret, MODE_RMDIR);
}

#endif

#else

SEC("kprobe/vfs_write")
__u64 kprobe_vfs_write(struct pt_regs *ctx)
{
	struct file *file = (struct file *)PT_REGS_PARM1(ctx);
	__u64 count = (__u64)PT_REGS_PARM3(ctx);

	return handle_rw_event(file, (__s64)count, MODE_WRITE);
}

SEC("kprobe/vfs_read")
__u64 kprobe_vfs_read(struct pt_regs *ctx)
{
	struct file *file = (struct file *)PT_REGS_PARM1(ctx);
	__u64 count = (__u64)PT_REGS_PARM3(ctx);

	return handle_rw_event(file, (__s64)count, MODE_READ);
}

SEC("kprobe/vfs_writev")
__u64 kprobe_vfs_writev(struct pt_regs *ctx)
{
	struct file *file = (struct file *)PT_REGS_PARM1(ctx);
	__u64 count = (__u64)PT_REGS_PARM3(ctx);

	return handle_rw_event(file, (__s64)count, MODE_WRITE);
}

SEC("kprobe/vfs_readv")
__u64 kprobe_vfs_readv(struct pt_regs *ctx)
{
	struct file *file = (struct file *)PT_REGS_PARM1(ctx);
	__u64 count = (__u64)PT_REGS_PARM3(ctx);

	return handle_rw_event(file, (__s64)count, MODE_READ);
}

SEC("kretprobe/vfs_create")
__u64 kretprobe_vfs_create(struct pt_regs *ctx)
{
	__s64 ret = (__s64)PT_REGS_RC(ctx);

	return handle_inode_event((__s64)ret, MODE_CREATE);
}

SEC("kretprobe/vfs_open")
__u64 kretprobe_vfs_open(struct pt_regs *ctx)
{
	__s64 ret = (__s64)PT_REGS_RC(ctx);

	return handle_inode_event((__s64)ret, MODE_OPEN);
}

SEC("kretprobe/vfs_mkdir")
__u64 kretprobe_vfs_mkdir(struct pt_regs *ctx)
{
	__s64 ret = (__s64)PT_REGS_RC(ctx);

	return handle_inode_event((__s64)ret, MODE_MKDIR);
}

SEC("kretprobe/vfs_unlink")
__u64 kretprobe_vfs_unlink(struct pt_regs *ctx)
{
	__s64 ret = (__s64)PT_REGS_RC(ctx);

	return handle_inode_event((__s64)ret, MODE_UNLINK);
}

SEC("kretprobe/vfs_rmdir")
__u64 kretprobe_vfs_rmdir(struct pt_regs *ctx)
{
	__s64 ret = (__s64)PT_REGS_RC(ctx);

	return handle_inode_event((__s64)ret, MODE_RMDIR);
}

#endif
