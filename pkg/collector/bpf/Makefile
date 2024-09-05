.PHONY: all clean
.SUFFIXES:

include ./Makefile.common

VFSDIR := vfs/
NETWORKDIR := network/
BPFTESTDIR := test/

NETWORK = bpf_network.o
VFS = bpf_vfs.o bpf_vfs_v511.o bpf_vfs_v62.o
# BPFTEST = bpf_lseek.o

OBJSDIR       := objs/
DEPSDIR       := deps/

VFSOBJ        := $(addprefix $(OBJSDIR),$(VFS))
NETWORKOBJ    := $(addprefix $(OBJSDIR),$(NETWORK))
TESTOBJ       := $(addprefix $(OBJSDIR),$(BPFTEST))
OBJS          := $(VFSOBJ) $(NETWORKOBJ) $(TESTOBJ)
LLOBJS        := $(patsubst $(OBJSDIR)%.o,$(OBJSDIR)%.ll,$(OBJS))
DEPS          := $(patsubst $(OBJSDIR)%.ll,$(DEPSDIR)%.d,$(LLOBJS))

all: $(OBJS) $(DEPS)

# NB: https://www.gnu.org/software/make/manual/html_node/Prerequisite-Types.html
$(OBJS): | $(OBJSDIR)
$(DEPS): | $(DEPSDIR)
$(LLOBJS): | $(OBJSDIR)

$(OBJSDIR):
	mkdir $(OBJSDIR)

$(DEPSDIR):
	mkdir $(DEPSDIR)

define DEFINE_VARIANT
VAR := $1
deps/bpf_vfs_$$(VAR).d: vfs/bpf_vfs.c
endef

# Generic build targets for each sub-dir

$(eval $(call DEFINE_VARIANT,v511))
$(eval $(call DEFINE_VARIANT,v62))

# Build only for relevant architectures
ifeq ($(BPF_TARGET_COMPILE),1)

# VFSDIR
objs/%.ll: $(VFSDIR)%.c
	$(CLANG) $(CLANG_FLAGS) -D__KERNEL_POST_v62 -c $< -o $@

objs/%_v511.ll:
	$(CLANG) $(CLANG_FLAGS) -D__KERNEL_PRE_v511 -c $< -o $@

objs/%_v62.ll:
	$(CLANG) $(CLANG_FLAGS) -D__KERNEL_POST_v512_PRE_v62 -c $< -o $@

$(DEPSDIR)%.d: $(VFSDIR)%.c
	$(CLANG) $(CLANG_FLAGS) -D__KERNEL_POST_v62 -MM -MP -MT $(patsubst $(DEPSDIR)%.d, $(OBJSDIR)%.ll, $@)   $< > $@

$(DEPSDIR)%_v511.d:
	$(CLANG) $(CLANG_FLAGS) -D__KERNEL_PRE_v511 -MM -MP -MT $(patsubst $(DEPSDIR)%.d, $(OBJSDIR)%.ll, $@)   $< > $@

$(DEPSDIR)%_v62.d:
	$(CLANG) $(CLANG_FLAGS) -D__KERNEL_POST_v512_PRE_v62 -MM -MP -MT $(patsubst $(DEPSDIR)%.d, $(OBJSDIR)%.ll, $@)   $< > $@

# NETWORKDIR
objs/%.ll: $(NETWORKDIR)%.c
	$(CLANG) $(CLANG_FLAGS) -c $< -o $@

$(DEPSDIR)%.d: $(NETWORKDIR)%.c
	$(CLANG) $(CLANG_FLAGS) -MM -MP -MT $(patsubst $(DEPSDIR)%.d, $(OBJSDIR)%.ll, $@)   $< > $@

# BPFTESTDIR
objs/%.ll: $(BPFTESTDIR)%.c
	$(CLANG) $(CLANG_FLAGS) -c $< -o $@

$(DEPSDIR)%.d: $(BPFTESTDIR)%.c
	$(CLANG) $(CLANG_FLAGS) -MM -MP -MT $(patsubst $(DEPSDIR)%.d, $(OBJSDIR)%.ll, $@)   $< > $@

# Remaining objects are built without mcpu=v2
objs/%.o: objs/%.ll
	$(LLC) $(LLC_FLAGS) -filetype=obj $< -o $@

# include dependencies, see https://lists.gnu.org/archive/html/make-w32/2004-03/msg00062.html
ifeq (,$(filter $(MAKECMDGOALS),clean run-test))
-include $(DEPS)
endif

endif

# the 'test' target is already taken
run-test:
	$(MAKE) -C tests test

# SUBDIRS=tests

clean:
	@$(ECHO_CLEAN)
	$(QUIET) $(foreach TARGET,$(SUBDIRS), \
		$(MAKE) -C $(TARGET) clean)
	$(QUIET)rm -f $(OBJSDIR)*.{o,ll,i,s}
	$(QUIET)rm -f $(DEPSDIR)*.d 
